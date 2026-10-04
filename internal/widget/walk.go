// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// Walk visits w and every widget nested beneath it, in schema order, recursing
// through each widget's children seam. Tree passes that must cover the whole
// schema — the localization walk here, the datatype gate in the server — share
// this one recursion instead of each maintaining its own container list, so a
// new container widget extends every pass by implementing children() once.
func Walk(w Widget, visit func(Widget)) {
	if w == nil {
		return
	}
	visit(w)
	for _, c := range w.children() {
		Walk(c, visit)
	}
}

// WalkActive visits controls that participate in this submission. Conditional
// fieldsets remain in the schema for rendering and translation, but an inactive
// branch is disabled in the browser and must not reject a save on the server.
func WalkActive(w Widget, visit func(Widget)) {
	if w == nil {
		return
	}
	if branch, ok := w.(*When); ok && !branch.Active {
		return
	}
	visit(w)
	for _, child := range w.children() {
		WalkActive(child, visit)
	}
}

// pruner is the removing half of the children() seam: a widget holding arbitrary
// child widgets rewrites its own slices, keeping only what keep accepts and
// recursing into the survivors. Walk visits a tree but cannot change it, and a
// rule that has to make a widget not exist needs the difference. Widgets whose
// only attachment is a typed badge or link (a callout's link, a row's status)
// implement nothing: nothing removable can reach them.
type pruner interface {
	prune(keep func(Widget) bool)
}

// pruneList filters one child slice and recurses into every widget that survives
// — the body every container's prune method delegates to.
func pruneList(ws []Widget, keep func(Widget) bool) []Widget {
	kept := ws[:0]
	for _, w := range ws {
		if !keep(w) {
			continue
		}
		if p, ok := w.(pruner); ok {
			p.prune(keep)
		}
		kept = append(kept, w)
	}
	return kept
}

// HasLiveListing reports whether the page lists something whose rows arrive
// while it is read — the one place the page-wide lens earns its place. A page
// that holds what it has is read by scrolling and found in with the browser's
// own find, however long it is; a live one keeps changing under that find, so
// only a lens of its own can hold a question across the rows to come. The rule
// lives here rather than in each plugin: what a filter is worth is a property
// of the vocabulary, not of any one domain.
func HasLiveListing(w Widget) bool {
	live := false
	Walk(w, func(n Widget) {
		if t, ok := n.(*Table); ok && t.streaming() {
			live = true
		}
	})
	return live
}

// StripFilters removes every filter widget from the tree and returns the root —
// nil when the root was the filter itself. Removal, not concealment: a lens the
// page did not earn leaves no markup and no sticky dock behind.
func StripFilters(w Widget) Widget {
	keep := func(n Widget) bool { _, isFilter := n.(*Filter); return !isFilter }
	if w == nil || !keep(w) {
		return nil
	}
	if p, ok := w.(pruner); ok {
		p.prune(keep)
	}
	return w
}

// firstFilter returns the first Filter anywhere in the tree, or nil.
func firstFilter(w Widget) *Filter {
	var found *Filter
	Walk(w, func(n Widget) {
		if found == nil {
			if f, ok := n.(*Filter); ok {
				found = f
			}
		}
	})
	return found
}

// HoistFilter moves a page-wide Filter to the body's first child, wherever a page
// placed it in its tree. The still-lens is a sticky dock the stylesheet positions
// from the body's first child; declared anywhere else it renders in flow and floats
// mid-page. Hoisting makes placement the shell's job, not a convention each page has
// to remember — a page declares the lens, the shell docks it below the masthead. A
// tree with no Filter is returned unchanged.
func HoistFilter(w Widget) Widget {
	if w == nil {
		return nil
	}
	if _, ok := w.(*Filter); ok {
		return w // the body is the filter itself; nothing to reposition
	}
	filter := firstFilter(w)
	if filter == nil {
		return w
	}
	root := StripFilters(w)
	if root == nil {
		return filter
	}
	// A page's root is a vertical Stack; joining the filter as its first child
	// reproduces exactly the structure a correctly-authored page has, so one
	// stylesheet rule docks every page's lens. A non-Stack root is wrapped.
	if s, ok := root.(*Stack); ok {
		s.Children = append([]Widget{Widget(filter)}, s.Children...)
		return s
	}
	return &Stack{Children: []Widget{filter, root}}
}

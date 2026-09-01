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

// PageFormCount counts the page-form surfaces a tree renders: a form declared
// style "page" and a reorderable table's hidden order form. The capsule binds
// to exactly one page form; a page composing more mis-wires silently, so the
// gateway holds the renderer to one and logs the composition that breaks it.
func PageFormCount(w Widget) int {
	count := 0
	Walk(w, func(n Widget) {
		switch n := n.(type) {
		case *Form:
			if n.Style == "page" {
				count++
			}
		case *Table:
			if n.reorderable() {
				count++
			}
		}
	})
	return count
}

// FilterThreshold is how much a page has to list before the page-wide lens earns
// its place. At or below it the whole page is one glance, and a search field over
// it is a control answering a question nobody asked — so the shell removes it.
// The rule lives here rather than in each plugin: what a filter is worth is a
// property of the vocabulary, not of any one domain.
const FilterThreshold = 20

// FilterableCount counts what the lens would have to sift: every table row, the
// folded ones included (a seam opens when it holds a match), and every settings
// option row, the folded ones likewise. Those are the entries the filter dims and
// reveals; nothing else on a page is filterable.
func FilterableCount(w Widget) int {
	count := 0
	Walk(w, func(n Widget) {
		switch n := n.(type) {
		case *Table:
			count += len(n.Rows)
			if n.Seam != nil {
				count += len(n.Seam.Rows)
			}
		case *Settings:
			count += len(n.Items)
			if n.Seam != nil {
				count += len(n.Seam.Items)
			}
		}
	})
	return count
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

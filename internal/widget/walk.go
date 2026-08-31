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

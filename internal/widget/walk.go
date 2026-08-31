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

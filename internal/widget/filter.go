// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Filter is the page-wide lens: one field that narrows every listing on the
// page at once, because the questions people ask are cross-section ("what
// touches guest?", "what opens :53?"). The plugin declares only the
// placeholder; the shell owns the behaviour (verso.js) and the still-lens
// contract — while typing, nothing moves: non-matching rows dim in place,
// zero-match sections ghost whole, and a table seam opens only when it holds a
// match. It rides a sticky ground-coloured dock; on pin the bar breathes in a
// step. "/" focuses it from anywhere; Escape clears.
type Filter struct {
	Placeholder string `json:"placeholder,omitempty"`
}

func (*Filter) isWidget() {}

func (f *Filter) renderInto(r *Renderer, out io.Writer, _ string) error {
	ph := f.Placeholder
	if ph == "" {
		ph = r.tr("Filter…")
	}
	return r.execute(out, "filter.html.tmpl", ph)
}

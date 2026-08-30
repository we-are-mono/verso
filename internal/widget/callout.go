// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Callout is a boxed notice: an icon, an optional title, and a line of body text,
// coloured by a semantic variant (never a colour, ADR-005). It is how a page tells
// the person something they need to know in context — a warning that the VPN isn't
// reachable, a confirmation that it is, a hint. Compact is the one-line form for a
// short contextual note. The plugin-emittable companion to the shell's own banners.
type Callout struct {
	Variant string `json:"variant"` // "info" (default) | "neutral" | "success" | "warning" | "danger"
	Title   string `json:"title"`
	Body    string `json:"body"`
	Compact bool   `json:"compact,omitempty"` // tighter, body-only treatment for a short contextual note
}

func (*Callout) isWidget() {}

func (c *Callout) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "callout.html.tmpl", c)
}

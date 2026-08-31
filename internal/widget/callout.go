// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"html/template"
	"io"
)

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
	Link    *Link  `json:"link,omitempty"`
}

func (*Callout) isWidget() {}

func (c *Callout) children() []Widget {
	if c.Link == nil {
		return nil
	}
	return []Widget{c.Link}
}

func (c *Callout) renderInto(r *Renderer, out io.Writer, _ string) error {
	var link template.HTML
	if c.Link != nil {
		var buf bytes.Buffer
		if err := c.Link.renderInto(r, &buf, ""); err != nil {
			return err
		}
		link = template.HTML(buf.String())
	}
	return r.execute(out, "callout.html.tmpl", struct {
		*Callout
		RenderedLink template.HTML
	}{Callout: c, RenderedLink: link})
}

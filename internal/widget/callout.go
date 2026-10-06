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
	// Verbatim is what a tool reported, in its own words: it rides inside the
	// band under a hairline of the band's tone, in mono, with a copy control,
	// because a message cut short or paraphrased is a message withheld.
	Verbatim string `json:"verbatim,omitempty"`
	// Acts resolve what the notice says (install what is missing, go where
	// it is fixed): buttons in a row inside the band, a cell under its words,
	// in the band's own ink.
	Acts []Link `json:"acts,omitempty"`
}

func (*Callout) isWidget() {}

func (c *Callout) children() []Widget {
	var out []Widget
	if c.Link != nil {
		out = append(out, c.Link)
	}
	for i := range c.Acts {
		out = append(out, &c.Acts[i])
	}
	return out
}

func (c *Callout) renderInto(r *Renderer, out io.Writer, _ string) error {
	render := func(l *Link) (template.HTML, error) {
		var buf bytes.Buffer
		if err := l.renderInto(r, &buf, ""); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil //nolint:gosec // rendered by the shell's own templates
	}
	var link template.HTML
	if c.Link != nil {
		var err error
		if link, err = render(c.Link); err != nil {
			return err
		}
	}
	acts := make([]template.HTML, 0, len(c.Acts))
	for i := range c.Acts {
		act, err := render(&c.Acts[i])
		if err != nil {
			return err
		}
		acts = append(acts, act)
	}
	return c.execute(r, out, link, acts)
}

// execute draws the band around what is already rendered for it: the link
// that closes its words and the acts in its row. A confirmation asks its
// question through it, its answers as the acts.
func (c *Callout) execute(r *Renderer, out io.Writer, link template.HTML, acts []template.HTML) error {
	return r.execute(out, "callout.html.tmpl", struct {
		*Callout
		RenderedLink template.HTML
		RenderedActs []template.HTML
	}{Callout: c, RenderedLink: link, RenderedActs: acts})
}

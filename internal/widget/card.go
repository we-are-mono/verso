// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
)

// Card is a titled container that nests other widgets. It is the composition
// primitive: the schema's ability to nest — not merely render a leaf — rests on it.
// Children decode recursively through Decode, so a card may hold any widget, itself
// included, and the closed set is enforced at every level. An optional Subtitle sits
// just under the title as part of the header block — describing the card — set apart
// from the content below it.
type Card struct {
	Style    string  `json:"style"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle"`
	Children Widgets `json:"children"`
}

func (*Card) isWidget() {}

func (c *Card) children() []Widget { return c.Children }

func (c *Card) prune(keep func(Widget) bool) { c.Children = pruneList(c.Children, keep) }

// cardView is the card template's model: the title and subtitle plus the children
// already rendered to trusted HTML fragments.
type cardView struct {
	Style    string
	Title    string
	Subtitle string
	Children []template.HTML
}

// renderInto renders each child through the renderer, so composition/nesting lives
// in Go and the template stays a dumb shell. The CSRF token flows to any nested form.
func (c *Card) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(c.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "card.html.tmpl", cardView{Style: c.Style, Title: c.Title, Subtitle: c.Subtitle, Children: children})
}

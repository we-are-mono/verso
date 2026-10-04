// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
)

// Disclosure is an expand/collapse section: a summary line the person clicks to
// reveal the contents beneath. It is the home for the advanced-but-usually-untouched
// (server keys, ports) — present and honest, but folded away so it never crowds the
// common path. Native <details>, so it is pure HTML: no JavaScript.
//
// Open renders it already expanded — for content that is the page's focus right
// now (an update's package manifest) yet still folds away once read.
type Disclosure struct {
	Style    string  `json:"style,omitempty"`
	Summary  string  `json:"summary"`
	Open     bool    `json:"open,omitempty"`
	Children Widgets `json:"children"`
}

func (*Disclosure) isWidget() {}

func (d *Disclosure) children() []Widget { return d.Children }

func (d *Disclosure) prune(keep func(Widget) bool) { d.Children = pruneList(d.Children, keep) }

// disclosureView is the template's model: the summary plus the contents already
// rendered to trusted HTML.
type disclosureView struct {
	Style    string
	Summary  string
	Open     bool
	Children []template.HTML
}

func (d *Disclosure) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(d.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "disclosure.html.tmpl", disclosureView{Style: d.Style, Summary: d.Summary, Open: d.Open, Children: children})
}

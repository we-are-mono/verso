// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Disclosure is an expand/collapse section: a summary line the person clicks to
// reveal the contents beneath. It is the home for the advanced-but-usually-untouched
// (server keys, ports) — present and honest, but folded away so it never crowds the
// common path. Native <details>, so it is pure HTML: no JavaScript.
type Disclosure struct {
	Style    string   `json:"style,omitempty"`
	Summary  string   `json:"summary"`
	Children []Widget `json:"children"`
}

func (*Disclosure) isWidget() {}

func (d *Disclosure) children() []Widget { return d.Children }

// UnmarshalJSON decodes the contents recursively through Decode, so an unknown
// child type fails loudly rather than vanishing.
func (d *Disclosure) UnmarshalJSON(data []byte) error {
	var raw struct {
		Style    string            `json:"style"`
		Summary  string            `json:"summary"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Style, d.Summary = raw.Style, raw.Summary
	children, err := decodeChildren(raw.Children, "disclosure child")
	if err != nil {
		return err
	}
	d.Children = children
	return nil
}

// disclosureView is the template's model: the summary plus the contents already
// rendered to trusted HTML.
type disclosureView struct {
	Style    string
	Summary  string
	Children []template.HTML
}

func (d *Disclosure) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(d.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "disclosure.html.tmpl", disclosureView{Style: d.Style, Summary: d.Summary, Children: children})
}

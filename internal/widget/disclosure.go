// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Disclosure is an expand/collapse section: a summary line the person clicks to
// reveal the contents beneath. It is the home for the advanced-but-usually-untouched
// (server keys, ports) — present and honest, but folded away so it never crowds the
// common path. Native <details>, so it is pure HTML: no JavaScript.
type Disclosure struct {
	Summary  string   `json:"summary"`
	Children []Widget `json:"children"`
}

func (*Disclosure) isWidget() {}

// UnmarshalJSON decodes the contents recursively through Decode, so an unknown
// child type fails loudly rather than vanishing.
func (d *Disclosure) UnmarshalJSON(data []byte) error {
	var raw struct {
		Summary  string            `json:"summary"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Summary = raw.Summary
	d.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("disclosure child %d: %w", i, err)
		}
		d.Children = append(d.Children, w)
	}
	return nil
}

// disclosureView is the template's model: the summary plus the contents already
// rendered to trusted HTML.
type disclosureView struct {
	Summary  string
	Children []template.HTML
}

func (d *Disclosure) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(d.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "disclosure.html.tmpl", disclosureView{Summary: d.Summary, Children: children})
}

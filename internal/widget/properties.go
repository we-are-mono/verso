// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Properties is a label/value detail list (a description list): each row shows a
// name on the left and its value on the right. It is the read-only companion to a
// form — the way to lay out the facts about one thing (a device's address, data
// used, when it was added) so they line up and scan cleanly.
//
// Style sets how rows are separated: a hairline between them ("divided", the default),
// zebra shading ("striped"), or nothing ("plain"). The shell owns the chrome; the
// template maps the style to classes, and any unknown value falls back to "divided".
type Properties struct {
	Style string     `json:"style"` // "divided" (default) | "striped" | "plain"
	Items []Property `json:"items"`
}

// Property is one row: a label, its value, whether the value is monospaced (for
// addresses, keys, and other machine text), and whether to offer an inline copy
// button beside the value (for values a person needs to paste elsewhere).
type Property struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Mono  bool   `json:"mono"`
	Copy  bool   `json:"copy"`
	// Chip renders the value as the small category chip — the same treatment
	// a zone gets everywhere else, so one fact never wears two dresses.
	Chip bool `json:"chip,omitempty"`
}

func (*Properties) isWidget() {}

func (p *Properties) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "properties.html.tmpl", p)
}

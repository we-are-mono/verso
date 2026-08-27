// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Properties is a label/value detail list (a description list): each row shows a
// name on the left and its value on the right. It is the read-only companion to a
// form — the way to lay out the facts about one thing (a device's address, data
// used, when it was added) so they line up and scan cleanly.
//
// Style sets how rows are separated: a hairline between them ("divided", the
// default) or nothing ("plain"). The shell owns the chrome; the template maps the
// style to classes, and any unknown value (including the retired "striped") falls
// back to "divided".
type Properties struct {
	Style string     `json:"style"` // "divided" (default) | "plain"
	Items []Property `json:"items"`
	// Align: "" keeps values on the right edge (the default fact sheet);
	// "left" sets them beside a fixed-width label column — the reading order
	// for values a person compares line by line (addresses), with any status
	// pill still holding the right edge.
	Align string `json:"align,omitempty"`
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
	// Status hangs a trailing state pill after the value — the same badge
	// vocabulary rows and pill cells speak.
	Status *Badge `json:"status,omitempty"`
}

func (*Properties) isWidget() {}

func (p *Properties) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "properties.html.tmpl", p)
}

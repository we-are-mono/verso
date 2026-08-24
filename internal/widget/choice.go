// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Choice is a single pick from a few rich options, shown as radio-cards — each with
// a title and a plain-language description, so the person chooses by meaning, not by
// a bare value. It is the humane form of a select when the options deserve a sentence
// of explanation (e.g. what a device should be allowed to reach). Pure CSS
// (ADR-005 §7): the radio's :checked state lights the selected card, so it needs no
// JavaScript.
type Choice struct {
	Name    string         `json:"name"`  // form field name the pick posts under
	Label   string         `json:"label"` // optional question shown above the cards
	Options []ChoiceOption `json:"options"`
}

// ChoiceOption is one radio-card: a value, a title, an optional description, and
// whether it starts selected.
type ChoiceOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Desc    string `json:"desc"`
	Checked bool   `json:"checked"`
}

func (*Choice) isWidget() {}

func (c *Choice) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "choice.html.tmpl", c)
}

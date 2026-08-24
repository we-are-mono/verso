// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Field is one labelled form control. Its props are semantic (ADR-005): kind and
// datatype express intent, never presentation. Value/Error carry the round-trip
// state — a plugin re-renders the field with the submitted Value and an Error on
// a failed POST (ADR-006 §5).
type Field struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`     // "text" (default) | "select" | "password" | "textarea"
	Value    string   `json:"value"`    // current/submitted value
	Datatype string   `json:"datatype"` // tier-1 datatype name, e.g. "hostname"
	Options  []Option `json:"options"`  // choices when kind is "select"
	Error    string   `json:"error"`    // inline validation error (set on 422)
	Help     string   `json:"help"`     // optional helper text
}

// Option is one choice in a select field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func (*Field) isWidget() {}

func (f *Field) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "field.html.tmpl", f)
}

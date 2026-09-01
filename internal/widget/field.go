// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Field is one labelled form control. Its props are semantic (ADR-005): kind and
// datatype express intent, never presentation. Value/Error carry the round-trip
// state — a plugin re-renders the field with the submitted Value and an Error on
// a failed POST (ADR-006 §5).
//
// Advanced marks a field as part of the advanced reading only (ADR-015). It is a
// boolean rather than a section's three-state mode because a field has no simpler
// face — a simplified representation is structural, which is what a section is
// for. The declaring plugin drops the flag for a field whose value differs from
// its default, so mode hides capability and never state.
type Field struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Kind         string   `json:"kind"`                   // "text" (default) | "select" | "checks" | "password" | "file" | "textarea" | "datetime-local" | "hidden"
	Autocomplete string   `json:"autocomplete,omitempty"` // optional browser autofill purpose, e.g. "current-password"
	Accept       string   `json:"accept,omitempty"`       // kind "file": native accepted file types/extensions
	Prompt       string   `json:"prompt,omitempty"`       // kind "file": sentence before the shell-owned picker link
	Placeholder  string   `json:"placeholder,omitempty"`  // text input hint; never substitutes for a visible label where one is required
	Autofocus    bool     `json:"autofocus,omitempty"`    // focus this field when its task-specific page opens
	Advanced     bool     `json:"advanced,omitempty"`     // part of the advanced reading only (ADR-015)
	Required     bool     `json:"required,omitempty"`
	Value        string   `json:"value"`    // current/submitted value
	Values       []string `json:"values"`   // kind "checks": the checked option values
	Datatype     string   `json:"datatype"` // tier-1 datatype name, e.g. "hostname"
	Options      []Option `json:"options"`  // choices when kind is "select" or "checks"
	Error        string   `json:"error"`    // inline validation error (set on 422)
	Help         string   `json:"help"`     // optional helper text
}

// Option is one choice in a select or checks field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func (*Field) isWidget() {}

func (*Field) children() []Widget { return nil }

// Checked reports whether v is among the checks field's current values — the
// template's membership test. Selection semantics per the control vocabulary:
// checks = "include this one" (a set), never on/off state (that's a switch).
func (f *Field) Checked(v string) bool {
	for _, cur := range f.Values {
		if cur == v {
			return true
		}
	}
	return false
}

func (f *Field) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "field.html.tmpl", f)
}

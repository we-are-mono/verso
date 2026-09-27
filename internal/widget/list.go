// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// List is a repeating text field: several values under one name, each validated
// against the same datatype. It is the schema's answer to LuCI's DynamicList — a
// stress test for the static-schema bet: repetition and per-item validation, not a
// single leaf.
//
// It renders each item as an input plus one trailing blank slot; all share Name,
// so they post as a multi-value form field. The plugin reads them, drops blanks,
// validates each, and rewrites the list. Per-item errors come back keyed by the
// item's index (as a string), which is how a repeating widget reports *which*
// row failed.
type List struct {
	Name     string            `json:"name"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind"`     // item kind; "text" for now
	Style    string            `json:"style"`    // "" repeated inputs | "tokens" compact removable values
	Prompt   string            `json:"prompt"`   // token input placeholder
	Datatype string            `json:"datatype"` // tier-1 datatype for each item
	Items    []string          `json:"items"`
	Errors   map[string]string `json:"errors"` // index (string) -> error
	Help     string            `json:"help"`
	// Key is the option this list writes, verbatim, for the mono chip beside the
	// label — a list of values is as much a line of the config as a single one.
	Key string `json:"key,omitempty"`
	// Target is where Key lives, "config.section", as a field's is; Staged is
	// the shell's word that the list's option waits to be applied.
	Target string `json:"target,omitempty"`
	Staged bool   `json:"-"`
	// Tip is the longer explanation, raised onto the label as a field's is.
	Tip string `json:"tip,omitempty"`
	// Options are the values worth offering: the control suggests them as you
	// type and lets several be chosen, while still accepting anything the config
	// accepts. A closed choice would be wrong here — fw4 reads protocol names
	// and numbers alike — but a list with no suggestions makes the operator
	// remember what the daemon happens to call things.
	Options []Option `json:"options,omitempty"`
	// Remove is the row's trailing remove affordance, as a field's is: "yes"
	// draws the glyph that clears the row, "lane" reserves its width.
	Remove string `json:"remove,omitempty"`
}

// Removable and Lane mirror a field's, so a list sits in a set of rows the same
// way every other row does.
func (l *List) Removable() bool { return l.Remove == "yes" }
func (l *List) Lane() bool      { return l.Remove != "" }

// ControlMeasure gives network lists the same editing measure as a single
// value, including their values, remove buttons and add control.
func (l *List) ControlMeasure() string {
	if l.Key == "listen_http" || l.Key == "listen_https" {
		return "endpoint"
	}
	return networkControlMeasure(l.Datatype)
}

// Explained reports whether the label carries an explanation to raise.
func (l *List) Explained() bool { return l.Tip != "" || l.Help != "" }

// LabelView is this list's left column — the same shape every other row's is.
func (l *List) LabelView() fieldLabel {
	tip := (&Field{Name: l.Name, Key: l.Key, Tip: l.Tip, Help: l.Help}).TipView()
	return fieldLabel{
		For: l.Name, Label: l.Label, Key: l.Key,
		Explained: l.Explained(), Tip: tip, Staged: l.Staged,
	}
}

func (*List) isWidget() {}

func (*List) children() []Widget { return nil }

func (l *List) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.renderFrame(out, "list.control", l, fieldFrame{
		Label:   l.LabelView(),
		Change:  fieldChange{Track: true, Name: l.Name, Label: l.Label, Kind: "list"},
		Measure: l.ControlMeasure(), Lane: l.Lane(), Removable: l.Removable(),
	})
}

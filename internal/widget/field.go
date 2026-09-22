// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"io"
	"strings"
)

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
	Kind         string   `json:"kind"`                   // "text" (default) | "select" | "checks" | "password" | "file" | "textarea" | "time" | "datetime-local" | "hidden"
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
	Help         string   `json:"help"`     // the sentence explaining the field; raised onto the label, see Tip
	// Key is the option this field writes, verbatim — "ipaddr", "leasetime".
	// It rides beside the label as a mono chip so someone who knows the config
	// can see which line they are editing without leaving the form, and someone
	// who does not can ignore it: the label is still the label.
	Key string `json:"key,omitempty"`
	// Tip explains what the field is to someone meeting it for the first time,
	// on hovering or focusing the label. It is the deliberate version of Help:
	// where both are set the label raises this one, because a plugin's Help was
	// written as a line under a control and a tip has room for the longer
	// answer to "what even is this". A field that needs no such answer carries
	// neither and the label is inert.
	Tip string `json:"tip,omitempty"`
	// Source names what reads the option — "dhcp host", "ip route". With Key it
	// makes the tip's footer, so the explanation ends by placing the option in
	// the config it belongs to rather than leaving it floating.
	Source string `json:"source,omitempty"`
	// Unit is what the number in the box is counted in — "Mbit/s", "seconds".
	// It sits inside the field's trailing edge, quiet and inert, so the value
	// and what it means read as one thing and the label above is left to say
	// what the setting is rather than how it is spelled.
	Unit string `json:"unit,omitempty"`
	// Pair makes this row two values with a word between them — a window
	// ("09:00 to 17:00"), a date range. They are one setting asked once, so they
	// are one row: as two rows a reader is invited to set the start and forget
	// the end, which is a rule that does the opposite of what they meant.
	Pair *FieldPair `json:"pair,omitempty"`
	// Remove is the row's trailing remove affordance, for a form whose rows are
	// a set someone adds to and takes from rather than a fixed list of settings:
	// "yes" draws the glyph that clears this row, "lane" reserves its width on a
	// row that cannot be removed, and "" draws neither. The lane matters as much
	// as the glyph — without it the rows that cannot be removed put their
	// controls 28px left of the ones that can, and a column that nearly lines up
	// reads worse than one that plainly does not.
	Remove string `json:"remove,omitempty"`
	// Style is the control's compact face where its option set is short enough
	// to show whole: "" is the ordinary control; "segmented" draws a kind
	// "checks" field as one strip of togglable chips — days of the week, address
	// families — instead of a grid of boxes. A set long enough to wrap belongs in
	// the grid, which is why this is the caller's call and not the widget's.
	Style string `json:"style,omitempty"`
}

// Segmented reports whether a field draws as one compact strip rather than as
// its ordinary control: a checks field as togglable chips, a select as the whole
// choice laid out. Either way the set has to be short enough to show whole.
func (f *Field) Segmented() bool { return f.Style == "segmented" }

// Removable reports whether this row draws the glyph that clears it, and Lane
// whether it reserves that glyph's width either way. A removable row reserves it
// by being it, which is why the two are asked separately.
func (f *Field) Removable() bool { return f.Remove == "yes" }
func (f *Field) Lane() bool      { return f.Remove != "" }

// Explained reports whether the label carries an explanation to raise. A field
// that only has Help carries one too: the sentence explaining a setting belongs
// on the thing it explains rather than in a line under the control, where it
// pushes every row apart and turns a form of eight settings into a page of
// prose. Tip is the same sentence said deliberately; Help is the same sentence
// a plugin already wrote.
func (f *Field) Explained() bool { return f.Tip != "" || f.Help != "" }

// DescribedBy is what the control is described by: the explanation raised onto
// its label, then the refusal under it. A screen reader reads both when the
// field takes focus, so the help is not only for someone who hovers.
func (f *Field) DescribedBy() string {
	ids := make([]string, 0, 2)
	if f.Explained() {
		ids = append(ids, f.TipView().ID)
	}
	if f.Error != "" {
		ids = append(ids, f.Name+"-error")
	}
	return strings.Join(ids, " ")
}

// explanation is the sentence the label raises: the explicit tip where there is
// one, and the help line otherwise.
func (f *Field) explanation() string {
	if f.Tip != "" {
		return f.Tip
	}
	return f.Help
}

// fieldLabel is a form row's left column: what the setting is called, the option
// it writes, and the explanation the label raises. Every widget that is one
// setting builds one of these, so a row's left half is one shape whatever
// control ends it — a value to type and a state to flip are the same question
// asked of the same object, and a form that draws them differently reads as two
// forms stacked.
type fieldLabel struct {
	For       string
	Label     string
	Key       string
	Explained bool
	Tip       TipView
}

// LabelView is this field's left column.
func (f *Field) LabelView() fieldLabel {
	return fieldLabel{
		For: f.Name, Label: f.Label, Key: f.Key,
		Explained: f.Explained(), Tip: f.TipView(),
	}
}

// TipView is one wide tooltip's model — the sentence, the line that places it in
// the config, and the id the thing it explains points at.
type TipView struct {
	ID     string
	Tip    string
	Footer string
}

// TipView is the label's explanation as the shared tooltip renders it. The
// footer is the option and what reads it, joined the way Verso joins a pair of
// machine words everywhere else; an option named without a reader, or a reader
// without an option, says whichever half it has.
func (f *Field) TipView() TipView {
	parts := make([]string, 0, 2)
	for _, part := range []string{f.Key, f.Source} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return TipView{ID: f.Name + "-tip", Tip: f.explanation(), Footer: strings.Join(parts, " · ")}
}

// FieldPair is the second half of a paired row: the value that closes the range
// the field's own value opens, and the word that joins them. Key states both
// options on the row's one chip, because the row is one setting.
type FieldPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Join  string `json:"join"`
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

// Measure keeps short machine values at the measure their grammar needs.
func (f *Field) Measure() string {
	if f.Kind == "select" || f.Kind == "checks" || f.Kind == "password" || f.Kind == "textarea" {
		return "full"
	}
	switch f.Datatype {
	case "ip4addr", "ip6addr", "ipaddr":
		return "address"
	case "port":
		return "number"
	}
	switch f.Key {
	case "mtu", "metric", "vid", "start", "limit", "ip6assign", "Port", "port", "listen_http", "listen_https":
		return "number"
	case "ipaddr", "ip6addr", "peeraddr", "peer6addr", "netmask", "gateway", "macaddr", "ula_prefix", "leasetime":
		return "address"
	}
	return "full"
}

// Words reports whether what is typed into this field is words rather than a
// machine string, and so is set in the sans at 14px rather than in mono: a
// password, a description, any free-text box whose value has no grammar (no
// datatype, no machine measure, no unit) such as a rule's name, and a select
// whose options are phrases rather than the config's own values. Everything
// with a grammar is an identifier and stays mono.
func (f *Field) Words() bool {
	switch f.Kind {
	case "password", "textarea":
		return true
	case "", "text":
		return f.Datatype == "" && f.Unit == "" && f.Pair == nil && f.Measure() == "full"
	case "select":
		// A select is set like what it offers: when every option shows its own
		// value the choices are the config's verbatim strings (a zone, an
		// interface, accept), and any phrase written for people makes it words.
		for _, o := range f.Options {
			if o.Label != o.Value {
				return true
			}
		}
	}
	return false
}

// LineCount is the code editor's live metadata, counting logical lines rather
// than treating the conventional final newline as another option.
func (f *Field) LineCount() int {
	if f.Value == "" {
		return 0
	}
	return len(strings.Split(strings.TrimSuffix(f.Value, "\n"), "\n"))
}

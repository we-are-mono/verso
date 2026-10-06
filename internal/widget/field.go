// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"io"
	"strings"
)

// RadioOptionLimit is the largest single-choice option set shown as radios.
// Larger sets use a native dropdown. This policy applies to pages and drawers;
// set it to zero to use dropdowns for every single-choice field.
const RadioOptionLimit = 3

// Field is one labelled form control. Its props are semantic (ADR-005): kind and
// datatype express intent, never presentation. Value/Error carry the round-trip
// state — a plugin re-renders the field with the submitted Value and an Error on
// a failed POST (ADR-006 §5).
type Field struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`                   // "text" (default) | "select" | "checks" | "password" | "file" | "textarea" | "time" | "datetime-local" | "hidden"
	Autocomplete string `json:"autocomplete,omitempty"` // optional browser autofill purpose, e.g. "current-password"
	Accept       string `json:"accept,omitempty"`       // kind "file": native accepted file types/extensions
	Prompt       string `json:"prompt,omitempty"`       // kind "file": sentence before the shell-owned picker link
	// Chosen is the name of the file a "text" file field already read: with
	// that style the browser reads the chosen file and posts what it says as
	// the field's value, and its name as <name>_name, so a form drawn again
	// around it still says which file it holds.
	Chosen       string   `json:"chosen,omitempty"`
	Placeholder  string   `json:"placeholder,omitempty"` // text input hint; never substitutes for a visible label where one is required
	Autofocus    bool     `json:"autofocus,omitempty"`   // focus this field when its task-specific page opens
	Required     bool     `json:"required,omitempty"`
	Value        string   `json:"value"`    // current/submitted value
	Values       []string `json:"values"`   // kind "checks": the checked option values
	Datatype     string   `json:"datatype"` // tier-1 datatype name, e.g. "hostname"
	Options      []Option `json:"options"`  // choices when kind is "select" or "checks"
	Error        string   `json:"error"`    // inline validation error (set on 422)
	Help         string   `json:"help"`     // the sentence explaining the field; raised onto the label, see Tip
	HelpVerbatim bool     `json:"-"`        // shell-composed help already localized (a sentence filled with data)
	// Key is the option this field writes, verbatim — "ipaddr", "leasetime".
	// It rides beside the label as a mono chip so someone who knows the config
	// can see which line they are editing without leaving the form, and someone
	// who does not can ignore it: the label is still the label.
	Key string `json:"key,omitempty"`
	// Target is where Key lives, "config.section" — set when the form or
	// section around the field does not already say it. With Key it is the
	// option's full address, which is how the shell finds a field whose change
	// waits on the stage (MarkStaged) and marks it on every visit.
	Target string `json:"target,omitempty"`
	// Staged is the shell's word that this field's option waits to be applied.
	Staged bool `json:"-"`
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
	//
	// "locked" is a value fixed once written — a network's mode, a radio's
	// band: the row keeps its label and key, Value is shown in a quiet box with
	// a padlock and Help as the reason, and nothing posts.
	Style string `json:"style,omitempty"`
	// Reshapes marks a kind "select" whose value decides which fields the form
	// has — what kind of object a New drawer makes. Changing it asks the plugin
	// for the form again, with the values on screen and `_action=reshape`, and
	// the answer replaces the form where it stands; nothing is staged. A choice
	// that only shows or hides fields within one shape is a When, not this.
	Reshapes bool `json:"reshapes,omitempty"`
}

// UseRadios applies the shell's option-count policy, regardless of style hints
// in older plugin schemas. An empty option set stays an empty dropdown.
func (f *Field) UseRadios() bool {
	return f.Kind == "select" && len(f.Options) > 0 && len(f.Options) <= RadioOptionLimit
}

// SelectedIndex preserves native single-select behavior: use the current value
// when present, otherwise select the first option. Changing the presentation
// must not change which value the form submits before someone edits it.
func (f *Field) SelectedIndex() int {
	for i, option := range f.Options {
		if option.Value == f.Value {
			return i
		}
	}
	if len(f.Options) == 0 {
		return -1
	}
	return 0
}

// Segmented is a compact multi-select checkbox set. Single-choice presentation
// follows UseRadios, including schemas that still request the old strip.
func (f *Field) Segmented() bool { return f.Kind == "checks" && f.Style == "segmented" }

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
	if f.Unit != "" {
		ids = append(ids, f.Name+"-unit")
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
	For   string
	Group bool
	Label string
	Key   string
	// Mono sets a label that is a machine string (a path) in mono, as the
	// typography contract sets every verbatim string.
	Mono      bool
	Explained bool
	Tip       TipView
	// Staged draws the stage's mark beside the key: this setting's change
	// waits to be applied.
	Staged bool
}

// LabelView is this field's left column.
func (f *Field) LabelView() fieldLabel {
	return fieldLabel{
		For: f.Name, Group: f.UseRadios() && f.Style != "locked", Label: f.Label, Key: f.Key,
		Explained: f.Explained(), Tip: f.TipView(), Staged: f.Staged,
	}
}

// TipView is one wide tooltip's model — the sentence, the line that places it in
// the config, and the id the thing it explains points at.
type TipView struct {
	ID     string
	Tip    string
	Footer string
	// Parts explain a fused control one part at a time, each under its name,
	// where the group has no one sentence of its own.
	Parts []TipPart
}

// TipPart is one part's explanation in a fused control's tip.
type TipPart struct {
	Label, Tip string
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
	if !f.framed() {
		return r.execute(out, "field.html.tmpl", f)
	}
	return r.renderFrame(out, "field.control", f, f.frame())
}

// framed reports whether this field is one setting's row. A hidden carrier is
// never drawn, and a file's drop area is its dialog's whole subject rather than
// a row among others.
func (f *Field) framed() bool {
	return f.Style == "code" || (f.Kind != "hidden" && f.Kind != "file")
}

// frame is this field's row. A locked value posts nothing, so it tracks no
// change; the code editor's value is a whole file, which no preview patches
// line by line.
func (f *Field) frame() fieldFrame {
	change := fieldChange{Track: f.Style != "locked", Name: f.Name, Label: f.Label, Kind: f.Kind, Writes: f.Key}
	switch {
	case f.Style == "code":
		change.Kind, change.Writes = "textarea", ""
	case change.Kind == "":
		change.Kind = "text"
	}
	frame := fieldFrame{
		Label: f.LabelView(), Change: change, Measure: f.ControlMeasure(),
		Lane: f.Lane(), Removable: f.Removable(),
	}
	if f.Error != "" {
		frame.Errors = []fieldError{{ID: f.Name + "-error", Text: f.Error}}
	}
	return frame
}

// Part is this field as the one part of its own row — named by the row's
// label, its change tracked by the row — for the controls a lone field and a
// group's part draw through the same partials (verso-select,
// verso-text-input).
func (f *Field) Part() boxPart { return boxPart{Field: f} }

// Box is this field as a box of one part: the value, and the unit it is
// counted in inside the same frame. The row around it tracks the change.
func (f *Field) Box() boxView {
	return boxView{Parts: []boxPart{{Field: f}}}
}

// ControlMeasure names a reusable control width from the field's semantics.
// It does not change the plugin schema or the input typography selected by
// Measure and Words. The same measures apply on pages and in drawers.
func (f *Field) ControlMeasure() string {
	if f.Kind == "select" && !f.UseRadios() {
		return "choice"
	}
	if f.Kind == "password" && f.Style != "locked" {
		return "secret"
	}
	if (f.Kind != "" && f.Kind != "text") || f.Style == "locked" || f.Style == "code" {
		return ""
	}
	switch f.Key {
	case "username":
		// A sign-in's name stands at its password's width: one short pair.
		return "secret"
	case "ula_prefix":
		return "prefix"
	case "hostname", "domain":
		return "host"
	case "resolver_url":
		return "endpoint"
	}
	return networkControlMeasure(f.Datatype)
}

// networkControlMeasure keeps fields and lists at the same editing width.
// Compound DNS rules need room for both a domain and an address, or a URL path.
func networkControlMeasure(datatype string) string {
	switch datatype {
	case "host", "hostname", "fqdn", "dnsserver":
		return "host"
	case "dnsaddress", "dnsforward":
		return "endpoint"
	}
	return ""
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
	case "timehhmmss", "dateyyyymmdd":
		return "short"
	}
	switch f.Key {
	case "mtu", "metric", "vid", "start", "limit", "ip6assign", "Port", "port", "listen_http", "listen_https", "maxassoc", "cachesize", "dhcpleasemax", "synflood_rate", "synflood_burst", "limit_burst":
		return "number"
	case "ipaddr", "ip6addr", "peeraddr", "peer6addr", "netmask", "gateway", "macaddr", "ula_prefix", "leasetime":
		return "address"
	}
	return "full"
}

// Words reports whether what is typed into this field is words rather than a
// machine string, and so is set in the sans at 14px rather than in mono: a
// password, a description, any free-text box whose value has no grammar (no
// datatype, no machine measure, no unit) such as a rule's name, and any select,
// which is an enumerated choice unless its field declares a machine datatype.
// Everything with a grammar is an identifier and stays mono.
func (f *Field) Words() bool {
	switch f.Kind {
	case "password", "textarea":
		return true
	case "", "text":
		return f.Datatype == "" && f.Unit == "" && f.Measure() == "full"
	case "select":
		// A select is an enumerated choice, and a choice is words (a timezone,
		// a policy, a protocol), however its labels are spelled. Only a field
		// that declares a machine-string datatype picks a value someone would
		// retype verbatim, and that one is mono.
		return f.Datatype == ""
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

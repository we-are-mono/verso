// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
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
	Chosen      string `json:"chosen,omitempty"`
	Placeholder string `json:"placeholder,omitempty"` // text input hint; never substitutes for a visible label where one is required
	// Default is the value the daemon falls back to while the box is empty,
	// and Example a value of the kind the box takes. The shell words either
	// in the empty box ("default 150", "e.g. 1.1.1.1"), so the hint can never
	// pass for a value the field holds. A Placeholder of the plugin's own
	// stands as given.
	Default      string   `json:"default,omitempty"`
	Example      string   `json:"example,omitempty"`
	Autofocus    bool     `json:"autofocus,omitempty"` // focus this field when its task-specific page opens
	Required     bool     `json:"required,omitempty"`
	Value        string   `json:"value"`    // current/submitted value
	Values       []string `json:"values"`   // kind "checks": the checked option values
	Datatype     string   `json:"datatype"` // tier-1 datatype name, e.g. "hostname"
	Options      []Option `json:"options"`  // choices when kind is "select" or "checks"; on a text field, values it suggests (see Suggests)
	Error        string   `json:"error"`    // inline validation error (set on 422)
	Help         string   `json:"help"`     // the sentence explaining the field; raised onto the label, see Tip
	HelpVerbatim bool     `json:"-"`        // shell-composed help already localized (a sentence filled with data)
	// Note is what changing the field does, where that is worth knowing before
	// it is changed — "Takes effect immediately, for everyone who signs in".
	// It stays in view under the control, after any refusal, and the control
	// is described by it. What the field is belongs on the label (Help, Tip);
	// a note is the field's consequence, not its explanation.
	Note string `json:"note,omitempty"`
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
	// Staged is the shell's word that this field's option waits to be applied;
	// StagedAt is the option's address it was marked by, which its mark is
	// named by.
	Staged   bool   `json:"-"`
	StagedAt string `json:"-"`
	// Theme makes a select the Appearance card's theme pick (ADR-024): it
	// carries no name, so the form around it neither submits it nor counts it,
	// and the page posts a pick on its own (verso-forms.js). Shell only.
	Theme bool `json:"-"`
	// Fetch makes a select a choice of what to look at, not a setting: changed,
	// it fetches the view for its row's choices into the page and stages
	// nothing (the Temperatures graph's reading and window). Shell only, as
	// Theme is.
	Fetch *FieldFetch `json:"-"`
	// Wide stands the answer under its question at the row's full width, as
	// a fused answer too wide for the column does: free text read as a
	// sentence (the Pending changes page's note). Shell only.
	Wide bool `json:"-"`
	// HelpMarkdown reads the description as the shell's own Markdown, so a
	// page it names can stand in bold ("kept in **System › Changes**"),
	// through the same sanitizing renderer as every shell sentence. A
	// plugin's help stays the plain words it wrote. Shell only.
	HelpMarkdown bool `json:"-"`
	// OptionsVerbatim declares the options' labels names the machine or a
	// profile gave (a reading's "Memory", an SFP module's cage), which the
	// localization walk leaves as they are. Shell only.
	OptionsVerbatim bool `json:"-"`
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
// Suggests reports whether a text field offers values as it is typed into: its
// Options, each a value and what it is (a device's address and its name). They
// are a reminder rather than a menu — the box still takes whatever is typed —
// so a field that knows likely values need not make the operator fetch one
// from another page.
func (f *Field) Suggests() bool {
	return (f.Kind == "" || f.Kind == "text") && len(f.Options) > 0
}

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

// DescribedBy is what the control is described by: the description under its
// name, then the refusal under it, its note and its unit. A screen reader reads
// them when the field takes focus, folded or not.
func (f *Field) DescribedBy() string {
	ids := make([]string, 0, 2)
	if f.description() != "" {
		ids = append(ids, f.Name+"-desc")
	}
	if f.Error != "" {
		ids = append(ids, f.Name+"-error")
	}
	if f.Note != "" {
		ids = append(ids, f.Name+"-note")
	}
	if f.Unit != "" {
		ids = append(ids, f.Name+"-unit")
	}
	return strings.Join(ids, " ")
}

// chip is the option the row writes and what reads it, joined the way Verso
// joins a pair of machine words everywhere else; an option named without a
// reader, or a reader without an option, says whichever half it has.
func chip(key, source string) string {
	halves := make([]string, 0, 2)
	for _, half := range []string{key, source} {
		if half != "" {
			halves = append(halves, half)
		}
	}
	return strings.Join(halves, " · ")
}

// wide reports whether the field's control cannot stand in the answer column:
// a code or text area, an address of the endpoint measure, a weekday strip,
// or one the shell stands under its question (Wide).
func (f *Field) wide() bool {
	return f.Wide || f.Style == "code" || f.Kind == "textarea" || f.ControlMeasure() == "endpoint" || f.Segmented()
}

// description is what the field is for, written under its name: the help,
// then the tip where a plugin wrote both, so neither is dropped.
func (f *Field) description() string { return sentences(f.Help, f.Tip) }

// sentences joins what a row says about itself, skipping what is empty.
func sentences(parts ...string) string {
	said := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			said = append(said, part)
		}
	}
	return strings.Join(said, " ")
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
	// waits to be applied. StagedAt names the mark by the option's address.
	Staged   bool
	StagedAt string
}

// LabelView is this field's question: its name, its key and its mark. What
// the field is for stands under it as the row's description, not on it.
func (f *Field) LabelView() fieldLabel {
	return fieldLabel{
		For: f.Name, Group: f.UseRadios() && f.Style != "locked", Label: f.Label, Key: chip(f.Key, f.Source),
		Staged: f.Staged, StagedAt: f.StagedAt,
	}
}

// TipView is one wide tooltip's model — the sentence, the line that places it in
// the config, and the id the thing it explains points at. A condition's name
// raises one; a field's name raises none.
type TipView struct {
	ID     string
	Tip    string
	Footer string
}

// FieldFetch is where a fetching choice gets its view: URL answers it for the
// row's choices, which land in place of the element Target selects.
type FieldFetch struct {
	URL, Target string
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
	if f.Placeholder == "" && (f.Default != "" || f.Example != "") {
		hinted := *f
		hinted.Placeholder = emptyHint(r, f.Default, f.Example)
		f = &hinted
	}
	if !f.framed() {
		return r.execute(out, "field.html.tmpl", f)
	}
	frame := f.frame()
	if f.HelpMarkdown && frame.Desc != "" {
		said, err := r.inlineMarkdown(frame.Desc)
		if err != nil {
			return err
		}
		frame.DescHTML = said
	}
	return r.renderFrame(out, "field.control", f, frame)
}

// emptyHint words what an empty box says in place of a value: the daemon's
// fallback as a default, else a value of its kind as an example.
func emptyHint(r *Renderer, def, example string) string {
	if def != "" {
		return fmt.Sprintf(r.tr("default %s"), def)
	}
	if example != "" {
		return fmt.Sprintf(r.tr("e.g. %s"), example)
	}
	return ""
}

// framed reports whether this field is one setting's row. A hidden carrier is
// never drawn, and a file's drop area is its dialog's whole subject rather than
// a row among others.
func (f *Field) framed() bool {
	return f.Style == "code" || (f.Kind != "hidden" && f.Kind != "file")
}

// frame is this field's row. A locked value posts nothing, and a choice of
// what to look at stages nothing, so neither tracks a change; the code
// editor's value is a whole file, which no preview patches line by line. A
// choice of what to look at is no setting, so its row is plain.
func (f *Field) frame() fieldFrame {
	change := fieldChange{Track: f.Style != "locked" && f.Fetch == nil, Name: f.Name, Label: f.Label, Kind: f.Kind, Writes: f.Key}
	switch {
	case f.Style == "code":
		change.Kind, change.Writes = "textarea", ""
	case change.Kind == "":
		change.Kind = "text"
	}
	frame := fieldFrame{
		Label: f.LabelView(), Change: change, Measure: f.ControlMeasure(),
		Lane: f.Lane(), Removable: f.Removable(), Desc: f.description(), Note: f.Note, Wide: f.wide(),
		Plain: f.Fetch != nil,
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

// Measure keeps short machine values at the measure their grammar needs, wide
// enough for the hint its empty box shows: a hint is read in place of a value,
// so a box that cuts it ("default 1" for "default 150") misleads.
func (f *Field) Measure() string {
	measure := f.grammarMeasure()
	hint := utf8.RuneCountInString(f.Placeholder)
	if measure == "number" && hint > numberHintRunes {
		measure = "short"
	}
	if measure == "short" && hint > shortHintRunes {
		measure = "address"
	}
	return measure
}

// numberHintRunes and shortHintRunes are how long a hint each short measure
// holds whole, in the hint's sans, inside the box's padding.
const (
	numberHintRunes = 9
	shortHintRunes  = 13
)

// grammarMeasure is the measure a value's own grammar needs.
func (f *Field) grammarMeasure() string {
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
	case "mtu", "metric", "vid", "start", "limit", "ip6assign", "Port", "port", "listen_http", "listen_https", "maxassoc", "cachesize", "dhcpleasemax", "synflood_rate", "synflood_burst", "limit_burst", "check_interval", "force_interval", "retry_interval", "priority", "weight":
		return "number"
	case "ipaddr", "ip6addr", "peeraddr", "peer6addr", "netmask", "gateway", "macaddr", "ula_prefix", "leasetime", "dest_ip":
		return "address"
	case "dest_port", "src_dport":
		// A forward's port may be a range (8000-8010): short, and typed on a
		// keyboard with a hyphen rather than a numeric pad.
		return "short"
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"io"
	"strings"
)

// Switch is one persistent on/off setting. It uses the same control as table
// toggle cells, so an object's enabled state has one visual language in
// summaries and editors. Pure CSS in every style (ADR-005 §7): the checkbox's
// :checked state drives the box and, inline, the Label/OffLabel swap, so no
// style ships JavaScript.
type Switch struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	OffLabel string `json:"off_label,omitempty"`
	Help     string `json:"help,omitempty"`
	Style    string `json:"style,omitempty"` // "" labelled row | "checkbox" the same row | "inline" beside a section heading | "locked" a state nothing here changes: drawn set or clear, inert, posting nothing
	// Verbatim says the label is a machine string — a path, a device — rather
	// than words: it is set in mono and never looked up in a catalog.
	Verbatim bool `json:"verbatim,omitempty"`
	On       bool `json:"on,omitempty"`
	// Key is the option this switch writes, verbatim — "drop_invalid",
	// "flow_offloading". It rides beside the label as a mono chip, exactly as a
	// field's does: a state to flip is as much a line of the config as a value
	// to type, and the row that says so has to say it the same way.
	Key string `json:"key,omitempty"`
	// Tip is the longer answer to "what even is this", raised onto the label the
	// way a field's is. Help is the same sentence a plugin already wrote; where
	// both are set the label raises Tip.
	Tip string `json:"tip,omitempty"`
	// Source names what reads the option — "firewall defaults". With Key it makes
	// the tip's footer, placing the option in the config it belongs to.
	Source string `json:"source,omitempty"`
	// Target is where Key lives, "config.section", as a field's is; Staged is
	// the shell's word that the switch's option waits to be applied.
	Target string `json:"target,omitempty"`
	Staged bool   `json:"-"`
}

func (*Switch) isWidget() {}

func (*Switch) children() []Widget { return nil }

// Explained reports whether the label carries an explanation to raise — the same
// test a field applies, so the two rows agree about when a label becomes the
// thing you hover.
func (s *Switch) Explained() bool { return s.Tip != "" || s.Help != "" }

// LabelView is this switch's left column: the same shape a field's is, because
// it is the same row.
func (s *Switch) LabelView() fieldLabel {
	tip := (&Field{Name: s.Name, Key: s.Key, Source: s.Source, Tip: s.Tip, Help: s.Help}).TipView()
	return fieldLabel{
		For: s.Name, Label: s.Label, Key: s.Key, Mono: s.Verbatim,
		Explained: s.Explained(), Tip: tip, Staged: s.Staged,
	}
}

// locked reports whether the switch states a state nothing here changes.
func (s *Switch) locked() bool { return s.Style == "locked" }

// switchControl is what the shared checkbox (switch.control) draws: its state,
// the form name it posts under, and, when no label points at it, the name a
// screen reader announces. A switch in a table or settings row has no visible
// label of its own (the row is what it switches), so it borrows the row's name.
// A labelled switch leaves Label empty, because an aria-label would override the
// label it already has.
type switchControl struct {
	Name  string
	On    bool
	Label string
	// Described is the id of the explanation raised onto the row's label, so
	// the checkbox is read with it when it takes focus.
	Described string
	// Disabled draws the state inert: it is shown, never changed, never posted.
	Disabled bool
}

// Control is this switch's checkbox. Every style labels it (the form row's label
// points at it by id, the inline style wraps it with its words), so it carries
// no name of its own; an explained switch is described by its tip.
func (s *Switch) Control() switchControl {
	c := switchControl{Name: s.Name, On: s.On, Disabled: s.locked()}
	if s.Explained() {
		c.Described = s.Name + "-tip"
	}
	return c
}

func (s *Switch) renderInto(r *Renderer, out io.Writer, _ string) error {
	if s.Style == "inline" {
		return r.execute(out, "form_switch.html.tmpl", s)
	}
	return r.renderFrame(out, "switch.row", s.Control(), s.frame())
}

// frame is this switch's row: the checkbox before its label. A locked switch
// posts nothing, so it tracks no change.
func (s *Switch) frame() fieldFrame {
	return fieldFrame{
		Label:  s.LabelView(),
		Change: fieldChange{Track: !s.locked() && (s.Name != "" || s.Style == "checkbox"), Name: s.Name, Label: s.Label, Kind: "toggle"},
		Toggle: true,
	}
}

// grouped is this switch as one row of a labelled group (a switch group):
// the group's label names the option, so the row names only its own thing,
// and its help is its description, kept in view beside its siblings'.
func (s *Switch) grouped() (switchControl, fieldFrame) {
	control := switchControl{Name: s.Name, On: s.On, Disabled: s.locked()}
	frame := s.frame()
	frame.Label.Key = ""
	frame.Label.Explained = s.Tip != ""
	frame.Label.Tip.Tip = s.Tip
	if s.Tip != "" {
		control.Described = frame.Label.Tip.ID
	}
	if s.Help != "" {
		frame.Desc = s.Help
		control.Described = strings.TrimSpace(control.Described + " " + frame.DescID())
	}
	return control, frame
}

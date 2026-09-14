// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Switch is one persistent on/off setting. It uses the same control as table
// toggle cells, so an object's enabled state has one visual language in
// summaries and editors. The "hero" style is the reassuring lead for a page's
// one big state — a VPN, a guest network: a status beacon, a headline reading
// the current state in plain language (Label on, OffLabel off), an optional
// Meta sub-line, and a larger switch. Pure CSS in every style (ADR-005 §7):
// the checkbox's :checked state drives the track, the beacon, and the headline
// swap, so no style ships JavaScript.
type Switch struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	OffLabel string `json:"off_label,omitempty"`
	Help     string `json:"help,omitempty"`
	Style    string `json:"style,omitempty"` // "" labelled row | "inline" beside a section heading | "hero" the page's lead state
	Icon     string `json:"icon,omitempty"`  // hero beacon glyph: "shield" | "globe" | "device" (default)
	Meta     string `json:"meta,omitempty"`  // hero sub-line under the headline
	On       bool   `json:"on,omitempty"`
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
		For: s.Name, Label: s.Label, Key: s.Key,
		Explained: s.Explained(), Tip: tip,
	}
}

func (s *Switch) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "form_switch.html.tmpl", s)
}

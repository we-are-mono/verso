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
}

func (*Switch) isWidget() {}

func (*Switch) children() []Widget { return nil }

func (s *Switch) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "form_switch.html.tmpl", s)
}

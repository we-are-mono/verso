// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Toggle is a reassuring on/off switch: a status beacon, a headline that reads
// the current state in plain language, an optional sub-line, and the switch
// itself. It is the warm hero for anything a person turns on or off — a VPN, a
// guest network, port forwarding — so the page leads with "it's on" rather than
// a checkbox. Purely CSS-driven (ADR-005 §7): the checkbox's :checked state swaps
// the headline and the beacon, so it needs no JavaScript.
type Toggle struct {
	Icon     string `json:"icon"`      // beacon glyph: "shield" | "globe" | "device" (default)
	Label    string `json:"label"`     // headline shown when on, e.g. "Your home VPN is on"
	OffLabel string `json:"off_label"` // headline shown when off, e.g. "Your home VPN is off"
	Meta     string `json:"meta"`      // optional sub-line under the headline
	Name     string `json:"name"`      // form field name the switch posts under
	Checked  bool   `json:"checked"`   // initial state
}

func (*Toggle) isWidget() {}

func (t *Toggle) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "toggle.html.tmpl", t)
}

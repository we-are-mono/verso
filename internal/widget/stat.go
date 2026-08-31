// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Stat is a labelled big-number tile — a single metric shown for at-a-glance
// scanning: a small label, a large value (a number or a short word like "Online"),
// and optional unit, supporting line, and icon. It is a generic primitive, naming
// no domain: it composes any metric (a device count, a speed, an uptime, a
// temperature), the same way an HTML element would.
//
// The semantic variant tints only the icon and the status dot, never the value
// (ADR-005): the big number stays ink so figures read consistently across the UI,
// and state is carried by the accent, not by recolouring the number.
type Stat struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Unit    string `json:"unit"`    // optional small suffix on the value, e.g. "Mbps"
	Sub     string `json:"sub"`     // optional supporting line under the value
	Icon    string `json:"icon"`    // optional: globe | devices | speed | shield
	Variant string `json:"variant"` // "neutral" (default) | "good" | "warning" | "danger"
	Dot     bool   `json:"dot"`     // show a status dot on the sub line (pulses when good)
	Href    string `json:"href"`    // optional: makes the whole tile a doorway to a detail page
	// Style: "" wears the framed card; "bare" is label over number on the
	// open canvas — for tile rows inside panels that already carry a frame.
	Style string `json:"style,omitempty"`
	// Name is a stable handle for a live tile: the markup carries it (plus
	// value/unit hooks) so the shell's client script can stream fresh
	// readings in. A nameless stat is static.
	Name string `json:"name,omitempty"`
}

func (*Stat) isWidget() {}

func (*Stat) children() []Widget { return nil }

func (s *Stat) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "stat.html.tmpl", s)
}

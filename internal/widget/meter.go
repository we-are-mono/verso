// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
)

// Meter is a full donut ring for a single metric — memory, disk, CPU, a link speed, a
// data cap. It is a generic primitive that names no metric.
//
// The ring and the number carry different information, on purpose: the ring fills by
// Fill (a 0–100% proportion), and the centre shows Value in its native Unit. So the
// ring is the "how full" and the number never restates it — a storage ring reads
// "23 GB" over a ~72%-full ring, not "72%" over "23 GB of 32 GB". Fill can differ from
// Value (Value "23" GB with Fill 72), or match it (a CPU reading of "18" %, Fill 18).
// The shell colours the ring by how full it is (green/amber/red); Variant "info"
// overrides that with the accent, for a rate like speed that has no "getting full".
type Meter struct {
	Label   string `json:"label"`
	Value   string `json:"value"`   // the centred number in its native unit ("23", "1.2", "300", "18")
	Unit    string `json:"unit"`    // "GB" | "Mbps" | "%" | …
	Fill    int    `json:"fill"`    // ring fill, 0–100 percent (the proportion; may differ from Value)
	Detail  string `json:"detail"`  // the one fact worth acting on, e.g. "9 GB free"
	Variant string `json:"variant"` // "" auto-colour by Fill | "info" (accent, for a rate)
}

func (*Meter) isWidget() {}

// meterCircumference is the length of the ring (radius 50): 2·π·r.
const meterCircumference = 314.16

type meterView struct {
	Label  string
	Value  string
	Unit   string
	Detail string
	Band   string // "good" | "warn" | "danger" | "info"
	Dash   string // stroke-dasharray for the fill arc
}

func (m *Meter) renderInto(r *Renderer, out io.Writer, _ string) error {
	fill := m.Fill
	if fill < 0 {
		fill = 0
	} else if fill > 100 {
		fill = 100
	}
	band := "good"
	switch {
	case m.Variant == "info":
		band = "info"
	case fill >= 92:
		band = "danger"
	case fill >= 80:
		band = "warn"
	}
	dash := fmt.Sprintf("%.1f %.2f", float64(fill)/100*meterCircumference, meterCircumference)
	return r.execute(out, "meter.html.tmpl", meterView{Label: m.Label, Value: m.Value, Unit: m.Unit, Detail: m.Detail, Band: band, Dash: dash})
}

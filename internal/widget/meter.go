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
//
// Layout picks the face: the default ring, or "bar" — the same reading as a
// left-aligned strip (label, big value, a thin horizontal track, caption). Both
// faces carry the same live hooks and colour by the same band rule.
type Meter struct {
	Label   string `json:"label"`
	Value   string `json:"value"`   // the centred number in its native unit ("23", "1.2", "300", "18")
	Unit    string `json:"unit"`    // "GB" | "Mbps" | "%" | …
	Fill    int    `json:"fill"`    // ring fill, 0–100 percent (the proportion; may differ from Value)
	Detail  string `json:"detail"`  // the one fact worth acting on, e.g. "9 GB free"
	Variant string `json:"variant"` // "" auto-colour by Fill | "info" (accent, for a rate)
	Layout  string `json:"layout,omitempty"` // "" ring (default) | "bar" (horizontal strip)
	// Name is a stable handle for a live meter: the rendered markup carries it
	// (plus per-part hooks) so the shell's client script can stream fresh
	// readings into the ring and text in place. A nameless meter is static.
	Name string `json:"name,omitempty"`
}

func (*Meter) isWidget() {}

// meterCircumference is the length of the ring (radius 50): 2·π·r.
const meterCircumference = 314.16

type meterView struct {
	Label  string
	Value  string
	Unit   string
	Detail string
	Name   string
	Band   string // "good" | "warn" | "danger" | "info"
	Dash   string // stroke-dasharray for the ring's fill arc
	Width  string // width of the bar's fill, e.g. "72%"
}

// MeterBand is the ring's colour band for a fill: "good" until 80, "warn"
// until 92, "danger" beyond; Variant "info" overrides with the accent.
// Exported so a live reading's producer can send the same truth the renderer
// would have drawn.
func MeterBand(fill int, variant string) string {
	switch {
	case variant == "info":
		return "info"
	case fill >= 92:
		return "danger"
	case fill >= 80:
		return "warn"
	}
	return "good"
}

func (m *Meter) renderInto(r *Renderer, out io.Writer, _ string) error {
	fill := m.Fill
	if fill < 0 {
		fill = 0
	} else if fill > 100 {
		fill = 100
	}
	view := meterView{
		Label: m.Label, Value: m.Value, Unit: m.Unit, Detail: m.Detail,
		Name: m.Name, Band: MeterBand(fill, m.Variant),
		Dash:  fmt.Sprintf("%.1f %.2f", float64(fill)/100*meterCircumference, meterCircumference),
		Width: fmt.Sprintf("%d%%", fill),
	}
	tmpl := "meter.html.tmpl"
	if m.Layout == "bar" {
		tmpl = "meterbar.html.tmpl"
	}
	return r.execute(out, tmpl, view)
}

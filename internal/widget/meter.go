// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
)

// Meter is a horizontal gauge for a single metric — memory, disk, CPU, a link
// speed, a data cap: the label (with an optional icon), the big value in its
// native unit, a thin track filled to Fill, and the caption below. It is a
// generic primitive that names no metric.
//
// The track and the number carry different information, on purpose: the track
// fills by Fill (a 0–100% proportion), and the value shows the reading in its
// native Unit. So the track is the "how full" and the number never restates it —
// a storage meter reads "23 GB" over a ~72%-full track, not "72%" over
// "23 GB of 32 GB". Fill can differ from Value (Value "23" GB with Fill 72), or
// match it (a CPU reading of "18" %, Fill 18). The shell colours the track by
// how full it is (emerald/amber/red); Variant "info" overrides that with the
// accent, for a rate like speed that has no "getting full".
type Meter struct {
	Label   string `json:"label"`
	Value   string `json:"value"`   // the big number in its native unit ("23", "1.2", "300", "18")
	Unit    string `json:"unit"`    // "GB" | "Mbps" | "%" | …
	Fill    int    `json:"fill"`    // track fill, 0–100 percent (the proportion; may differ from Value)
	Detail  string `json:"detail"`  // the one fact worth acting on, e.g. "9 GB free"
	Variant string `json:"variant"` // "" auto-colour by Fill | "info" (accent, for a rate)
	// Icon sits in front of the label — a glyph naming the metric.
	Icon string `json:"icon,omitempty"`
	// Role paints the track (and the icon) a fixed decorative accent instead of
	// the fill-band health colour — for a dashboard row where each gauge carries
	// its own hue, not a health tone: "sky" | "violet" | "emerald" | "amber".
	Role string `json:"role,omitempty"`
	// Name is a stable handle for a live meter: the rendered markup carries it
	// (plus per-part hooks) so the shell's client script can stream fresh
	// readings into the track and text in place. A nameless meter is static.
	Name string `json:"name,omitempty"`
}

func (*Meter) isWidget() {}

func (*Meter) children() []Widget { return nil }

type meterView struct {
	Label  string
	Value  string
	Unit   string
	Detail string
	Name   string
	Icon   string
	Role   string // decorative accent for the track + icon; "" colours by band
	Band   string // the tone vocabulary: "success" | "warning" | "danger" | "info"
	Width  string // width of the track's fill, e.g. "72%"
}

// MeterBand is the track's colour band for a fill, in the tone vocabulary:
// "success" until 80, "warning" until 92, "danger" beyond; Variant "info"
// overrides with the accent. Exported so a live reading's producer can send
// the same truth the renderer would have drawn.
func MeterBand(fill int, variant string) string {
	switch {
	case variant == "info":
		return "info"
	case fill >= 92:
		return "danger"
	case fill >= 80:
		return "warning"
	}
	return "success"
}

func (m *Meter) renderInto(r *Renderer, out io.Writer, _ string) error {
	fill := m.Fill
	if fill < 0 {
		fill = 0
	} else if fill > 100 {
		fill = 100
	}
	return r.execute(out, "meter.html.tmpl", meterView{
		Label: m.Label, Value: m.Value, Unit: m.Unit, Detail: m.Detail,
		Name: m.Name, Icon: m.Icon, Role: m.Role, Band: MeterBand(fill, m.Variant),
		Width: fmt.Sprintf("%d%%", fill),
	})
}

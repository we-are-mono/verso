// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"io"
)

// Meter is a gauge for a single metric — memory, disk, CPU, a link speed, a
// data cap: the label (with an optional icon), a track of blocks lit to Fill,
// the value in its native unit, and the caption below. It is a generic
// primitive that names no metric.
//
// The track and the number carry different information, on purpose: the track
// lights by Fill (a 0–100% proportion), and the value shows the reading in its
// native Unit. So the track is the "how full" and the number never restates it —
// a storage meter reads "23 GB" over a ~72%-full track, not "72%" over
// "23 GB of 32 GB". Fill can differ from Value (Value "23" GB with Fill 72), or
// match it (a CPU reading of "18" %, Fill 18). The shell colours the lit blocks
// by how full the track is (emerald/amber/red); Variant "info" overrides that
// with the accent, for a rate like speed that has no "getting full".
type Meter struct {
	Overview bool   `json:"-"` // the Overview's gauge: its live hooks are the landing page's
	Label    string `json:"label"`
	// Verbatim declares the label an identity (a sensor's kernel channel name)
	// rather than prose — the localization walk leaves it exactly as authored.
	Verbatim bool   `json:"verbatim,omitempty"`
	Value    string `json:"value"`   // the big number in its native unit ("23", "1.2", "300", "18")
	Unit     string `json:"unit"`    // "GB" | "Mb/s" | "%" | …
	Fill     int    `json:"fill"`    // track fill, 0–100 percent (the proportion; may differ from Value)
	Detail   string `json:"detail"`  // the one fact worth acting on, e.g. "9 GB free"
	Variant  string `json:"variant"` // "" auto-colour by Fill | "info" (accent, for a rate)
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
	// Compact renders the meter as one dense row — the label, a track of ten
	// blocks, and the value with a status dot — for a headroom listing where
	// many readings stack. The track fills 0→the reading's own ceiling, so each
	// row measures against its own limit rather than a shared scale.
	Compact bool `json:"compact,omitempty"`
	// WarnMark is the reading's warn limit as a percent of its track (0 = none):
	// the blocks past it are shaded, so the track says where trouble starts
	// before the reading gets there. The track's end is its ceiling.
	WarnMark int `json:"warn_mark,omitempty"`
	// Tone colours the compact fill and its status dot by state — "success" |
	// "warning" | "danger" — since a temperature grades against its own limits,
	// not the fill band. "" leaves the fill on the calm accent.
	Tone string `json:"tone,omitempty"`
	// NoTrack drops the compact track — a reading with no limits shows a neutral
	// dot and its Detail as a quiet note, never a guessed bar.
	NoTrack bool `json:"no_track,omitempty"`
}

func (*Meter) isWidget() {}

func (*Meter) children() []Widget { return nil }

type meterView struct {
	Overview bool
	Label    string
	Value    string
	Unit     string
	Detail   string
	Name     string
	Icon     string
	Role     string // decorative accent for the track + icon; "" colours by band
	Band     string // the tone vocabulary: "success" | "warning" | "danger" | "info"
	// Track is the track's share and warn limit (meterTrack): the meter block
	// turns them into whole blocks, so a live reading changes one property
	// and never lays the page out again.
	Track   template.CSS
	Compact bool
	Tone    string // compact fill + dot colour: "success" | "warning" | "danger" | ""
	NoTrack bool
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

// meterTrack is the style a meter's track carries: the share it lights and,
// where the reading has one, its warn limit, each a percent of the track
// clamped to it. verso-stream.js writes the same share when a live reading
// lands.
func meterTrack(fill, warn int) template.CSS {
	style := fmt.Sprintf("--verso-meter-fill: %d", min(max(fill, 0), 100))
	if warn > 0 {
		style += fmt.Sprintf("; --verso-meter-warn: %d", min(warn, 100))
	}
	return template.CSS(style) //nolint:gosec // built here from clamped integers, never from input
}

func (m *Meter) renderInto(r *Renderer, out io.Writer, _ string) error {
	fill := min(max(m.Fill, 0), 100)
	return r.execute(out, "meter.html.tmpl", meterView{
		Overview: m.Overview,
		Label:    m.Label, Value: m.Value, Unit: m.Unit, Detail: m.Detail,
		Name: m.Name, Icon: m.Icon, Role: m.Role, Band: MeterBand(fill, m.Variant),
		Track:   meterTrack(fill, m.WarnMark),
		Compact: m.Compact, Tone: m.Tone, NoTrack: m.NoTrack,
	})
}

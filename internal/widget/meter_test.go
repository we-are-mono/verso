// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderMeter: the horizontal gauge — the value over its unit, a track
// filled to Fill%, and the caption.
func TestRenderMeter(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Storage", Value: "23", Unit: "GB", Fill: 72, Detail: "9 GB free"})
	for _, want := range []string{
		"Storage", ">23<", ">GB<", "9 GB free",
		"data-verso-meter-bar", // the fill element
		// filled by Fill: the bar spans the track and a clip shows the reading,
		// so a live update repaints the bar without laying the page out again
		"clip-path: inset(0 28% 0 0 round 9999px)",
		"bg-denim", // 72% is an ordinary quantity: the accent, not a warning
		"tabular-nums",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterBands: the track colours itself from Fill (not from the displayed Value).
func TestMeterBands(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{45: "bg-denim", 85: "bg-marigold", 95: "bg-crimson"}
	for fill, want := range cases {
		got := render(t, r, &Meter{Label: "x", Value: "x", Fill: fill})
		if !strings.Contains(got, want) {
			t.Errorf("fill %d: want %q in:\n%s", fill, want, got)
		}
	}
}

// TestMeterInfoVariant: "info" forces the accent and shows the value in its own unit,
// bypassing the fill-based auto-colour (a rate like speed has no "getting full").
// What it is asserting is that behaviour, not a colour of its own: the accent is the
// colour of any ordinary reading, and "info" means this one stays ordinary however
// full it looks — a 96% link is a fast link, not a warning.
func TestMeterInfoVariant(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Speed", Value: "300", Unit: "Mbps", Fill: 30, Variant: "info", Detail: "24 Mbps up"})
	for _, want := range []string{">300<", ">Mbps<", "bg-denim", "24 Mbps up"} {
		if !strings.Contains(got, want) {
			t.Errorf("speed meter missing %q:\n%s", want, got)
		}
	}
	full := render(t, r, &Meter{Label: "Speed", Value: "960", Unit: "Mbps", Fill: 96, Variant: "info"})
	if !strings.Contains(full, "bg-denim") {
		t.Errorf("an info reading near its ceiling still reads as a quantity:\n%s", full)
	}
	for _, never := range []string{"bg-crimson", "bg-marigold"} {
		if strings.Contains(full, never) {
			t.Errorf("info variant must not auto-colour, got %q:\n%s", never, full)
		}
	}
	if strings.Contains(got, "bg-marigold") || strings.Contains(got, "bg-crimson") {
		t.Errorf("info variant must not auto-colour:\n%s", got)
	}
}

// TestMeterLiveHooks: a named meter carries the handle and per-part hooks the
// shell's client script streams fresh readings into, and its track wears the
// transition class so a new width glides rather than snaps.
func TestMeterLiveHooks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Name: "memory", Label: "Memory", Value: "1.2", Unit: "GB", Fill: 60})
	for _, want := range []string{
		`data-verso-meter="memory"`,
		"data-verso-meter-bar", "verso-meter-bar",
		"data-verso-meter-value", "data-verso-meter-unit", "data-verso-meter-detail",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("live meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterNamelessIsStatic: without a Name there is no live handle, and an
// empty detail renders nothing at all.
func TestMeterNamelessIsStatic(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Storage", Value: "23", Unit: "GB", Fill: 72})
	if strings.Contains(got, `data-verso-meter="`) {
		t.Errorf("nameless meter must not carry a live handle:\n%s", got)
	}
	if strings.Contains(got, "data-verso-meter-detail") {
		t.Errorf("nameless meter with no detail renders no detail slot:\n%s", got)
	}
}

// TestMeterCompact: the dense headroom row — a track that fills to the reading's
// own critical, warn/critical tick marks, and a tone-coloured status dot.
func TestMeterCompact(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "Memory", Value: "47.0", Unit: "°C", Fill: 49, WarnMark: 89, CritMark: 100, Tone: "success"})
	for _, want := range []string{
		"Memory", ">47.0<", "°C",
		"clip-path: inset(0 51% 0 0 round 9999px)", // fills to the reading's own ceiling
		"left: 89%", // the warn tick
		// A reading in its ordinary range is a quantity, not a verdict, so the bar
		// is the action colour — what the canvas draws for every normal meter.
		// Green stays where the palette puts it: the dot beside the value.
		"bg-denim",
		"bg-green",
		"bg-marigold-deep", // the warn tick
		"bg-crimson",       // the critical tick
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterCompactNoTrack: a reading with no limits shows no track and a neutral
// dot — never a guessed threshold.
func TestMeterCompactNoTrack(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "Tctl", Value: "78", Unit: "°C", NoTrack: true})
	if strings.Contains(got, "data-verso-meter-bar") {
		t.Errorf("a no-limits reading must draw no track:\n%s", got)
	}
	for _, want := range []string{"no limits reported", "bg-glyph"} {
		if !strings.Contains(got, want) {
			t.Errorf("no-track meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterCompactWarnTone: a reading over its warn trip turns marigold, fill and
// dot together, and stops reading as an ordinary quantity.
func TestMeterCompactWarnTone(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "SoC", Value: "88", Unit: "°C", Fill: 92, WarnMark: 89, CritMark: 100, Tone: "warning"})
	if strings.Count(got, "bg-marigold\"") < 1 && !strings.Contains(got, "bg-marigold ") {
		t.Errorf("warm reading should tint marigold:\n%s", got)
	}
	for _, never := range []string{"bg-denim", "bg-green"} {
		if strings.Contains(got, never) {
			t.Errorf("warm reading must not keep %q, the ordinary-range colours:\n%s", never, got)
		}
	}
}

// The meter's colours are the palette's, in both faces of the widget. A stock
// Tailwind ramp here is a colour no design names — and the live layer repaints
// these same elements from verso-stream.js, so a ramp in one of them is a reading
// that changes colour the moment it updates.
func TestMeterUsesThePalette(t *testing.T) {
	r := newRenderer(t)
	for _, m := range []*Meter{
		{Compact: true, Label: "Memory", Value: "47", Unit: "°C", Fill: 49, WarnMark: 89, CritMark: 100, Tone: "success"},
		{Compact: true, Label: "Tctl", Value: "78", Unit: "°C", NoTrack: true},
		{Label: "Storage", Value: "2.1", Unit: "GB", Fill: 62},
	} {
		got := render(t, r, m)
		for _, ramp := range []string{"slate-", "gray-", "emerald-", "amber-", "red-", "sky-", "violet-"} {
			if strings.Contains(got, ramp) {
				t.Errorf("meter %q still carries the stock ramp %q:\n%s", m.Label, ramp, got)
			}
		}
	}
}

// TestMeterClampsFill: a Fill over 100 fills the whole track, not past it.
func TestMeterClampsFill(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "x", Value: "x", Fill: 150})
	if !strings.Contains(got, "clip-path: inset(0 0% 0 0 round 9999px)") {
		t.Errorf("fill>100 should fill the whole track:\n%s", got)
	}
}

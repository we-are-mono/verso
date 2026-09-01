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
		"width: 72%",           // filled by Fill
		"bg-emerald-600",       // 72% is still healthy (amber starts at 80)
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
	cases := map[int]string{45: "bg-emerald-600", 85: "bg-amber-500", 95: "bg-red-600"}
	for fill, want := range cases {
		got := render(t, r, &Meter{Label: "x", Value: "x", Fill: fill})
		if !strings.Contains(got, want) {
			t.Errorf("fill %d: want %q in:\n%s", fill, want, got)
		}
	}
}

// TestMeterInfoVariant: "info" forces the accent and shows the value in its own unit,
// bypassing the fill-based auto-colour (a rate like speed has no "getting full").
func TestMeterInfoVariant(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Speed", Value: "300", Unit: "Mbps", Fill: 30, Variant: "info", Detail: "24 Mbps up"})
	for _, want := range []string{">300<", ">Mbps<", "bg-sky-600", "24 Mbps up"} {
		if !strings.Contains(got, want) {
			t.Errorf("speed meter missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "bg-emerald-600") {
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
		"width: 49%",      // fills to the reading's own ceiling
		"left: 89%",       // the warn tick
		"bg-emerald-500",  // nominal tone on the fill + dot
		"bg-amber-500/60", // the warn tick colour
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
	for _, want := range []string{"no limits reported", "bg-slate-300"} {
		if !strings.Contains(got, want) {
			t.Errorf("no-track meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterCompactWarnTone: a reading over its warn trip colours the fill and dot
// amber, not the calm accent.
func TestMeterCompactWarnTone(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "SoC", Value: "88", Unit: "°C", Fill: 92, WarnMark: 89, CritMark: 100, Tone: "warning"})
	if !strings.Contains(got, "bg-amber-500") {
		t.Errorf("warm reading should tint amber:\n%s", got)
	}
	if strings.Contains(got, "bg-emerald-500") {
		t.Errorf("warm reading must not stay emerald:\n%s", got)
	}
}

// TestMeterClampsFill: a Fill over 100 fills the whole track, not past it.
func TestMeterClampsFill(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "x", Value: "x", Fill: 150})
	if !strings.Contains(got, "width: 100%") {
		t.Errorf("fill>100 should fill the whole track:\n%s", got)
	}
}

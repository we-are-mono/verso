// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// meterBar is the opening of a meter's bar in band (or, with no band, in the
// block's own green), up to its clip.
func meterBar(band string) string {
	if band == "" {
		return `class="verso-meter__bar" style="clip-path:`
	}
	return `class="verso-meter__bar" data-verso-tone="` + band + `"`
}

// meterBandFills is the colour the meter block paints each band's bar.
var meterBandFills = map[string]struct{ sel, decl string }{
	"success": {".verso-meter__bar", "background-color: var(--color-green);"},
	"warning": {`.verso-meter__bar[data-verso-tone="warning"]`, "background-color: var(--color-marigold);"},
	"danger":  {`.verso-meter__bar[data-verso-tone="danger"]`, "background-color: var(--color-crimson);"},
	"info":    {`.verso-meter__bar[data-verso-tone="info"]`, "background-color: var(--color-denim);"},
}

// declaresBandFill fails t unless the meter block paints band's bar in its
// colour.
func declaresBandFill(t *testing.T, band string) {
	t.Helper()
	fill := meterBandFills[band]
	declares(t, "meter", fill.sel, fill.decl)
}

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
		meterBar("success"), // 72% is in the ordinary range: the canvas's green band
		`<span class="verso-meter__figure" data-verso-meter-value>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter missing %q:\n%s", want, got)
		}
	}
	declaresBandFill(t, "success")
	// The figure is the figure role, whose digits are tabular.
	declares(t, "meter", ".verso-meter__figure", "@apply type-figure;")
	typeCSS, err := os.ReadFile(filepath.Join("..", "server", "assets", "type.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cssRule(string(typeCSS), "@utility type-figure"), "font-variant-numeric: tabular-nums;") {
		t.Error("the figure role's digits are not tabular")
	}
}

// TestMeterBands: the track colours itself from Fill (not from the displayed Value).
func TestMeterBands(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{45: "success", 85: "warning", 95: "danger"}
	for fill, band := range cases {
		got := render(t, r, &Meter{Label: "x", Value: "x", Fill: fill})
		if !strings.Contains(got, meterBar(band)) {
			t.Errorf("fill %d: want the %s band in:\n%s", fill, band, got)
		}
		declaresBandFill(t, band)
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
	for _, want := range []string{">300<", ">Mbps<", meterBar("info"), "24 Mbps up"} {
		if !strings.Contains(got, want) {
			t.Errorf("speed meter missing %q:\n%s", want, got)
		}
	}
	declaresBandFill(t, "info")
	full := render(t, r, &Meter{Label: "Speed", Value: "960", Unit: "Mbps", Fill: 96, Variant: "info"})
	if !strings.Contains(full, meterBar("info")) {
		t.Errorf("an info reading near its ceiling still reads as a quantity:\n%s", full)
	}
	for _, never := range []string{meterBar("danger"), meterBar("warning")} {
		if strings.Contains(full, never) {
			t.Errorf("info variant must not auto-colour, got %q:\n%s", never, full)
		}
	}
	if strings.Contains(got, meterBar("warning")) || strings.Contains(got, meterBar("danger")) {
		t.Errorf("info variant must not auto-colour:\n%s", got)
	}
}

// TestMeterLiveHooks: a named meter carries the handle and per-part hooks the
// shell's client script streams fresh readings into, and its bar eases to a
// new clip so a reading glides rather than snaps.
func TestMeterLiveHooks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Name: "memory", Label: "Memory", Value: "1.2", Unit: "GB", Fill: 60})
	for _, want := range []string{
		`data-verso-meter="memory"`,
		"data-verso-meter-bar", `class="verso-meter__bar"`,
		"data-verso-meter-value", "data-verso-meter-unit", "data-verso-meter-detail",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("live meter missing %q:\n%s", want, got)
		}
	}
	declares(t, "meter", ".verso-meter__bar", "transition: clip-path 600ms ease;")
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
		// A reading in its ordinary range is green, bar and dot alike — the
		// canvas's meter band — on a 6px track.
		`class="verso-meter" data-verso-place="row"`,
		`<span class="verso-meter__track">`,
		meterBar("success"),
		`<span class="verso-mark" data-verso-tone="success">`,
		// the warn tick, in the warn band's own hue, never past the critical one
		`class="verso-meter__tick" data-verso-tone="warning" style="left: min(89%, calc(100% - 0.375rem))"`,
		// the critical tick
		`class="verso-meter__tick" data-verso-tone="danger"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact meter missing %q:\n%s", want, got)
		}
	}
	declares(t, "meter", ".verso-meter__track", "height: calc(var(--spacing) * 1.5);", "border-radius: calc(infinity * 1px);", "background-color: var(--color-rule);")
	declaresBandFill(t, "success")
	declares(t, "meter", ".verso-meter__tick", "background-color: var(--color-marigold);")
	declares(t, "meter", `.verso-meter__tick[data-verso-tone="danger"]`, "background-color: var(--color-crimson);")
}

// TestMeterCompactNoTrack: a reading with no limits shows no track and a neutral
// dot — never a guessed threshold.
func TestMeterCompactNoTrack(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "Tctl", Value: "78", Unit: "°C", NoTrack: true})
	if strings.Contains(got, "data-verso-meter-bar") {
		t.Errorf("a no-limits reading must draw no track:\n%s", got)
	}
	for _, want := range []string{"no limits reported", `data-verso-tone="muted"`} {
		if !strings.Contains(got, want) {
			t.Errorf("no-track meter missing %q:\n%s", want, got)
		}
	}
	// Where the track would be, the reason there is none reads at the row's
	// 14px, led by the hollow square: a state with no hue of its own.
	if !strings.Contains(got, `<span class="verso-meter__note"><span class="verso-mark" aria-hidden="true"></span><span class="truncate">no limits reported</span></span>`) {
		t.Errorf("the missing track is not said at 14px behind a hollow square:\n%s", got)
	}
	declares(t, "meter", ".verso-meter__note", "color: var(--color-meta);")
	declares(t, "meter", `.verso-meter[data-verso-place="row"]`, "@apply type-body;")
	// In a page card on a phone the row keeps to the card: the name's column
	// narrows and the track's gives way, its words cut short rather than
	// pushing the page wider.
	if !strings.Contains(got, `class="verso-meter" data-verso-place="row"`) {
		t.Errorf("the reading's row is not the meter block's row:\n%s", got)
	}
	declares(t, "meter", `.verso-meter[data-verso-place="row"]`, "grid-template-columns: 7rem minmax(0, 1fr) 6.5rem;")
	_, wide, _ := strings.Cut(blockCSS(t, "meter"), "@media (width >= 40rem)")
	if !strings.Contains(cssRule(wide, `.verso-meter[data-verso-place="row"]`), "grid-template-columns: 9.5rem minmax(0, 1fr) 6.5rem;") {
		t.Error("the reading's row does not fit a phone's card: no wider name column past a phone")
	}
}

// TestMeterCompactWarnTone: a reading over its warn trip turns marigold, fill and
// dot together, and stops reading as an ordinary quantity.
func TestMeterCompactWarnTone(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "SoC", Value: "88", Unit: "°C", Fill: 92, WarnMark: 89, CritMark: 100, Tone: "warning"})
	for _, want := range []string{meterBar("warning"), `<span class="verso-mark" data-verso-tone="warning">`} {
		if !strings.Contains(got, want) {
			t.Errorf("warm reading should tint marigold, missing %q:\n%s", want, got)
		}
	}
	declaresBandFill(t, "warning")
	for _, never := range []string{meterBar("info"), meterBar("success"), `data-verso-tone="success"`} {
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

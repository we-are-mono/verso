// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"strings"
	"testing"
)

// meterBar is the opening of a meter's bar in band (or, with no band, in the
// block's own green, a bar with nothing after its class).
func meterBar(band string) string {
	if band == "" {
		return `class="verso-meter__bar"></span>`
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

// renderRaw renders w as the page receives it, the spaces between a caption's
// words and its figures kept (render normalizes them away).
func renderRaw(t *testing.T, r *Renderer, w Widget) string {
	t.Helper()
	var b strings.Builder
	if err := r.RenderWithToken(&b, w, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	return b.String()
}

// The gauge's rule: every meter that is neither a listing's row nor its cell.
const meterGauge = `.verso-meter:not([data-verso-place="row"], [data-verso-place="cell"])`

// TestRenderMeter: the gauge — its label, the track lit to Fill, the value in
// its own unit beside it, and the caption under the track.
func TestRenderMeter(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Storage", Value: "23", Unit: "GB", Fill: 72, Detail: "9 GB free"})
	for _, want := range []string{
		"Storage", ">23<", ">GB<", `<span class="verso-meter__number">9</span>`,
		"data-verso-meter-bar", // the fill element
		// The track carries the share it lights; the block turns it into
		// whole blocks, so a live reading changes one property and the page
		// is never laid out again.
		`class="verso-meter__track" style="--verso-meter-fill: 72"`,
		meterBar("success"), // 72% is in the ordinary range: the canvas's green band
		`<span class="verso-meter__figure" data-verso-figures data-verso-meter-value>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "clip-path") {
		t.Errorf("the reading rides on the track, not on a clip the template writes:\n%s", got)
	}
	declaresBandFill(t, "success")
	// A gauge is one line: its label at the start, the track and the figure
	// at the end, the caption under the track. Its label is a fact's label
	// and its figure the title's role, so the reading reads as a fact in a
	// list rather than a headline over a calm card.
	declares(t, "meter", meterGauge,
		"display: grid;",
		"grid-template-columns: calc(var(--spacing) * 20) minmax(0, 1fr) calc(var(--spacing) * 16);",
		`grid-template-areas: "head lane reading" ". detail detail";`,
		"column-gap: calc(var(--spacing) * 6);",
		"align-items: center;")
	declares(t, "meter", ".verso-meter__label", "color: var(--color-meta);", "@apply type-row-label;")
	declares(t, "meter", ".verso-meter__figure", "@apply type-title;")
	declares(t, "meter", ".verso-meter__unit", "@apply type-value;")
	declares(t, "meter", ".verso-meter__reading", "height: calc(var(--spacing) * 8);")
}

// TestMeterIsBlocks: every meter's track is one block, 10px by 16px and 3px
// apart, wherever it stands, as many as its lane has room for and never fewer
// than ten. The track is as long as its blocks, never stretched.
func TestMeterIsBlocks(t *testing.T) {
	declares(t, "meter", ".verso-meter",
		"--verso-meter-block: calc(var(--spacing) * 2.5);",
		"--verso-meter-gap: calc(var(--spacing) * 0.75);",
		"--verso-meter-step: calc(var(--verso-meter-block) + var(--verso-meter-gap));",
		"--verso-meter-blocks: 10;")
	// A block in its own right, so a row's or a cell's track (a span, in its
	// lane) keeps its size.
	declares(t, "meter", ".verso-meter__track",
		"display: block;",
		"position: relative;",
		"flex: none;",
		"width: calc(var(--verso-meter-blocks) * var(--verso-meter-step) - var(--verso-meter-gap));",
		"height: calc(var(--spacing) * 4);",
		// The range in Rule, its warn zone in Strong Hairline, both cut into
		// blocks by the mask.
		"background-image: linear-gradient(to right, var(--color-rule) calc(var(--verso-meter-zone) * var(--verso-meter-step)), var(--color-rule-strong) 0);",
		"mask-image: repeating-linear-gradient(to right, var(--color-ink) 0 var(--verso-meter-block), transparent 0 var(--verso-meter-step));")
	// A lit block is a whole block: the share rounds to the nearest, and any
	// reading above nothing lights one. The warn zone starts at the first
	// block that reaches past the warn limit; with no limit it has none.
	declares(t, "meter", ".verso-meter__track",
		"--verso-meter-lit: max(round(nearest, var(--verso-meter-fill, 0) * var(--verso-meter-blocks) / 100, 1), min(1, var(--verso-meter-fill, 0)));",
		"--verso-meter-zone: round(down, var(--verso-meter-warn, 100) * var(--verso-meter-blocks) / 100, 1);")
	declares(t, "meter", ".verso-meter__bar",
		"position: absolute;",
		"inset: 0;",
		"clip-path: inset(0 max(0px, calc(100% - var(--verso-meter-lit) * var(--verso-meter-step))) 0 0);")
	// The blocks stand square: no pill, and no ticks — the warn zone says the
	// limit, and the track's end is the protect limit.
	css := blockCSS(t, "meter")
	for _, gone := range []string{"verso-meter__tick", "9999px", "infinity"} {
		if strings.Contains(css, gone) {
			t.Errorf("blocks/meter.css still draws %q", gone)
		}
	}
}

// TestAGaugeFillsItsLane: a gauge's track holds as many blocks as its lane
// has room for, 10 to 25, so the label, the blocks and the figure stand the
// same distance apart at every width; on a phone's card the lane takes a
// line of its own under the label and figure.
func TestAGaugeFillsItsLane(t *testing.T) {
	got := render(t, newRenderer(t), &Meter{Label: "Storage", Value: "51", Unit: "%", Fill: 51})
	if !strings.Contains(got, `<div class="verso-meter__lane">`) {
		t.Errorf("the gauge's track does not stand in a lane:\n%s", got)
	}
	declares(t, "meter", ".verso-meter__lane", "grid-area: lane;", "container: verso-meter-lane / inline-size;", "min-width: 0;")
	css := blockCSS(t, "meter")
	// n blocks take n steps less one gap: 13n − 3px.
	for n := 11; n <= 25; n++ {
		at := fmt.Sprintf("@container verso-meter-lane (width >= %grem)", float64(13*n-3)/16)
		if !strings.Contains(cssRuleWithin(css, at, ".verso-meter__track"), fmt.Sprintf("--verso-meter-blocks: %d;", n)) {
			t.Errorf("%s: the lane does not hold %d blocks", at, n)
		}
	}
	if strings.Contains(css, "(width >= 20.9375rem)") {
		t.Error("a lane holds no more than 25 blocks")
	}
	stacked := cssRuleWithin(css, "@container card (width < 25rem)", meterGauge)
	for _, want := range []string{
		"grid-template-columns: minmax(0, 1fr) auto;",
		`grid-template-areas: "head reading" "lane lane" "detail detail";`,
	} {
		if !strings.Contains(stacked, want) {
			t.Errorf("a phone's gauge does not stack: missing %q in\n%s", want, stacked)
		}
	}
}

// TestMeterDetailWeighsItsFigures: in a gauge's caption a figure stands
// heavier than the words around it, its unit with it (the unit stands after
// its figure across a narrow space), so "34.1 of 60.4 GiB" is read by its
// quantities; a word with a digit in it ("1-minute") and a word after an
// ordinary space ("32 threads") stay words.
func TestMeterDetailWeighsItsFigures(t *testing.T) {
	r := newRenderer(t)
	for detail, want := range map[string]string{
		"34.1 of 60.4\U0000202fGiB · 26.3\U0000202fGiB free": `<span class="verso-meter__number">34.1</span> of <span class="verso-meter__number">60.4` + "\U0000202f" + `GiB</span> · <span class="verso-meter__number">26.3` + "\U0000202f" + `GiB</span> free`,
		"1-minute average · 32 threads":                      `1-minute average · <span class="verso-meter__number">32</span> threads`,
		"34,1 od 60,4\U0000202fGiB":                          `<span class="verso-meter__number">34,1</span> od <span class="verso-meter__number">60,4` + "\U0000202f" + `GiB</span>`,
	} {
		for _, m := range []*Meter{
			{Label: "Memory", Value: "56", Unit: "%", Fill: 56, Detail: detail},
			{Overview: true, Name: "sys-memory", Label: "Memory", Value: "56", Unit: "%", Fill: 56, Detail: detail},
		} {
			if got := renderRaw(t, r, m); !strings.Contains(got, "data-verso-meter-detail>"+want+"</div>") {
				t.Errorf("caption %q does not weigh its figures, want %q in:\n%s", detail, want, got)
			}
		}
	}
	declares(t, "meter", ".verso-meter__number", "@apply type-subheading;")
}

// TestMeterWarnZone: a reading with a warn limit shades the blocks past it,
// so the track says where trouble starts before the reading gets there.
func TestMeterWarnZone(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Memory", Value: "56", Unit: "%", Fill: 56, WarnMark: 75})
	if !strings.Contains(got, `style="--verso-meter-fill: 56; --verso-meter-warn: 75"`) {
		t.Errorf("the gauge does not carry its warn limit:\n%s", got)
	}
	plain := render(t, r, &Meter{Label: "Speed", Value: "300", Unit: "Mbps", Fill: 30, Variant: "info"})
	if strings.Contains(plain, "--verso-meter-warn") {
		t.Errorf("a reading with no warn limit shades no zone:\n%s", plain)
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
	for _, want := range []string{">300<", ">Mbps<", meterBar("info"), `<span class="verso-meter__number">24</span>Mbps up`} {
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
// new block so a reading steps rather than snaps.
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

// TestMeterCompact: the dense headroom row — ten blocks lit to the reading's
// own ceiling, the warn zone shaded past its warn limit, and a tone-coloured
// status dot.
func TestMeterCompact(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Compact: true, Label: "Memory", Value: "47.0", Unit: "°C", Fill: 49, WarnMark: 89, Tone: "success"})
	for _, want := range []string{
		"Memory", ">47.0<", "°C",
		// lit to the reading's own ceiling, its warn limit shading the zone, in
		// a lane filling the row's middle column
		`<span class="verso-meter__lane"><span class="verso-meter__track" style="--verso-meter-fill: 49; --verso-meter-warn: 89">`,
		// A reading in its ordinary range is green, bar and dot alike.
		`class="verso-meter" data-verso-place="row"`,
		meterBar("success"),
		`<span class="verso-mark" data-verso-tone="success">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact meter missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verso-meter__tick") {
		t.Errorf("the warn zone says the limit; the row draws no tick:\n%s", got)
	}
	declaresBandFill(t, "success")
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
	got := render(t, r, &Meter{Compact: true, Label: "SoC", Value: "88", Unit: "°C", Fill: 92, WarnMark: 89, Tone: "warning"})
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
		{Compact: true, Label: "Memory", Value: "47", Unit: "°C", Fill: 49, WarnMark: 89, Tone: "success"},
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

// TestMeterClampsFill: a Fill over 100 lights the whole track, not past it,
// and a warn limit past the track's end shades nothing.
func TestMeterClampsFill(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "x", Value: "x", Fill: 150, WarnMark: 140})
	if !strings.Contains(got, `style="--verso-meter-fill: 100; --verso-meter-warn: 100"`) {
		t.Errorf("fill>100 should light the whole track:\n%s", got)
	}
}

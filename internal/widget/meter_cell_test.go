// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func meterTable(cells ...TableCell) *Table {
	rows := make([]TableRow, len(cells))
	for i, c := range cells {
		rows[i] = TableRow{ID: string(rune('a' + i)), Cells: []TableCell{c}}
	}
	return &Table{Columns: []TableColumn{{Label: "Airtime busy", Kind: "meter", Width: MeasureName}}, Rows: rows}
}

// TestMeterCellDrawsTheShareAndItsFigure: a meter cell is the meter widget at
// a row's scale — a 6px track filled to the cell's share, and the figure beside
// it — so a listing can say how full each of its things is.
func TestMeterCellDrawsTheShareAndItsFigure(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "61 %", Fill: 61, Variant: "warning"}))
	for _, want := range []string{
		`<span class="verso-meter" data-verso-place="cell">`,
		`<span class="verso-meter__track">`,        // the canvas's meter track
		`clip-path: inset(0 39% 0 0 round 9999px)`, // filled to 61%, the widget's own clip
		meterBar("warning"),                        // the plugin's band, in the badge vocabulary
		`>61 %<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
	declares(t, "meter", ".verso-meter__track", "height: calc(var(--spacing) * 1.5);", "border-radius: calc(infinity * 1px);", "background-color: var(--color-rule);")
	declaresBandFill(t, "warning")
}

// TestMeterCellBandsFromTheVariant: the listing owns its thresholds — airtime
// is busy at a different share from storage — so the cell colours by the
// variant it is given: ordinary is green, warning marigold, danger crimson.
func TestMeterCellBandsFromTheVariant(t *testing.T) {
	r := newRenderer(t)
	for variant, band := range map[string]string{"": "success", "success": "success", "warning": "warning", "danger": "danger"} {
		got := render(t, r, meterTable(TableCell{Text: "40 %", Fill: 40, Variant: variant}))
		// The ordinary band is the bar's own green, so it names no tone.
		want := meterBar(band)
		if band == "success" {
			want = meterBar("")
		}
		if !strings.Contains(got, want) {
			t.Errorf("variant %q: want the %s band:\n%s", variant, band, got)
		}
		declaresBandFill(t, band)
	}
}

// TestMeterCellWithNoReadingIsADash: a thing that is off has no share to show,
// and an empty track would read as "idle" — the cell says nothing instead.
func TestMeterCellWithNoReadingIsADash(t *testing.T) {
	r := newRenderer(t)
	for _, cell := range []TableCell{{}, {Text: "—"}} {
		got := render(t, r, meterTable(cell))
		if strings.Contains(got, "clip-path") || strings.Contains(got, "verso-meter__track") {
			t.Errorf("a cell with no reading drew a track:\n%s", got)
		}
		if !strings.Contains(got, `text-faint"><span aria-hidden="true">—</span>`) {
			t.Errorf("a cell with no reading should be the faint dash:\n%s", got)
		}
	}
}

// TestMeterCellMarksItsLimits: a headroom listing (a temperature against the
// point the hardware protects itself) ticks its warn and protect limits on the
// track, as the compact meter does, so the bar says how far the reading is
// from each.
func TestMeterCellMarksItsLimits(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "48.0 °C", Fill: 50, WarnMark: 89, CritMark: 100}))
	for _, want := range []string{
		`class="verso-meter__tick" data-verso-tone="warning" style="left: min(89%, calc(100% - 0.375rem))"`, // the warn band's own hue, as its bar fills past it
		`class="verso-meter__tick" data-verso-tone="danger" style="left: calc(100% - 0.125rem)"`,
		`>48.0 °C<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
	declares(t, "meter", ".verso-meter__tick", "top: calc(var(--spacing) * -1);", "bottom: calc(var(--spacing) * -1);", "background-color: var(--color-marigold);")
	declares(t, "meter", `.verso-meter__tick[data-verso-tone="danger"]`, "background-color: var(--color-crimson);")
	// The ticks stand a pixel past the track above and below, so the track no
	// longer clips what it holds; the fill clips itself.
	css := blockCSS(t, "meter")
	cellTrack := `.verso-meter__track:where(.verso-meter:is([data-verso-place="row"], [data-verso-place="cell"]) *)`
	if rule := cssRule(css, cellTrack); !strings.Contains(rule, "position: relative;") || strings.Contains(rule, "overflow") {
		t.Errorf("the track clips its ticks:\n%s", rule)
	}
	if !strings.Contains(cssRule(css, `.verso-meter__track:where(.verso-meter:not([data-verso-place="row"], [data-verso-place="cell"]) *)`), "overflow: hidden;") {
		t.Error("only a gauge's track clips: the gauge's rule no longer excludes the cell")
	}

	// A warn limit at the protect limit stands just before its tick, 2px
	// apart, so every row reads marigold, then crimson; a warn limit with no
	// protect limit after it ends on the track's end, as a protect tick does.
	same := render(t, newRenderer(t), meterTable(TableCell{Text: "70.0 °C", Fill: 80, WarnMark: 100, CritMark: 100}))
	if !strings.Contains(same, `data-verso-tone="warning" style="left: min(100%, calc(100% - 0.375rem))"`) {
		t.Errorf("a warn limit at the protect limit should stand before it:\n%s", same)
	}
	alone := render(t, newRenderer(t), meterTable(TableCell{Text: "70.0 °C", Fill: 80, WarnMark: 100}))
	if !strings.Contains(alone, `data-verso-tone="warning" style="left: min(100%, calc(100% - 0.125rem))"`) {
		t.Errorf("a lone warn limit should end on the track's end:\n%s", alone)
	}
}

// TestMeterCellWithoutMarksIsUnchanged: a share (airtime, leases) has no
// limits, so its cell draws no tick.
func TestMeterCellWithoutMarksIsUnchanged(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "61 %", Fill: 61}))
	if strings.Contains(got, "verso-meter__tick") {
		t.Errorf("a meter cell with no marks drew a tick:\n%s", got)
	}
}

// TestMeterCellWithNoTrackKeepsItsFigure: a reading with no limits has no
// honest headroom, so it shows its figure in the figure's slot and no track,
// never a bar against a guessed ceiling.
func TestMeterCellWithNoTrackKeepsItsFigure(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "71.0 °C", NoTrack: true}))
	if strings.Contains(got, "clip-path") || strings.Contains(got, "verso-meter__track") {
		t.Errorf("a no-track cell drew a track:\n%s", got)
	}
	if !strings.Contains(got, `>71.0 °C<`) || strings.Contains(got, `aria-hidden="true">—<`) {
		t.Errorf("a no-track cell should show its figure, not the dash:\n%s", got)
	}
}

// TestMeterCellFigureHoldsAReadingWithItsUnit: the figure's slot is one width
// for every row, so the bars end together, and it holds a temperature with its
// unit on one line.
func TestMeterCellFigureHoldsAReadingWithItsUnit(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "100.0 °C", Fill: 100}))
	if !strings.Contains(got, `class="verso-meter__amount `) {
		t.Errorf("the figure's slot should be 4rem, one line:\n%s", got)
	}
	declares(t, "meter", ".verso-meter__amount", "flex-shrink: 0;", "width: calc(var(--spacing) * 16);", "text-align: right;", "white-space: nowrap;", "font-variant-numeric: tabular-nums;")
	// The track keeps a floor, so a narrow window scrolls the table rather
	// than shrinking the bar to a stub.
	if !strings.Contains(got, `<span class="verso-meter" data-verso-place="cell">`) || !strings.Contains(got, `<span class="verso-meter__track">`) {
		t.Errorf("the track should keep a minimum width:\n%s", got)
	}
	declares(t, "meter", `.verso-meter__track:where(.verso-meter[data-verso-place="cell"] *)`, "flex: 1 1 0%;", "min-width: calc(var(--spacing) * 16);")
}

// TestTextCellCarriesItsChip: a plain word can cite the config value it stands
// for — "WPA3" beside the `sae` it writes — in the same chip a name cell uses.
func TestTextCellCarriesItsChip(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Security"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "WPA3", Chip: "sae"}}}},
	})
	for _, want := range []string{">WPA3<", monoChip + "sae<"} {
		if !strings.Contains(got, want) {
			t.Errorf("text cell missing %q:\n%s", want, got)
		}
	}
}

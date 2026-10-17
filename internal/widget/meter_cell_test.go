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
		`h-1.5`, `rounded-full bg-rule`, // the canvas's meter track
		`clip-path: inset(0 39% 0 0 round 9999px)`, // filled to 61%, the widget's own clip
		`bg-marigold`, // the plugin's band, in the badge vocabulary
		`>61 %<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
}

// TestMeterCellBandsFromTheVariant: the listing owns its thresholds — airtime
// is busy at a different share from storage — so the cell colours by the
// variant it is given: ordinary is green, warning marigold, danger crimson.
func TestMeterCellBandsFromTheVariant(t *testing.T) {
	r := newRenderer(t)
	for variant, want := range map[string]string{"": "bg-green", "success": "bg-green", "warning": "bg-marigold", "danger": "bg-crimson"} {
		got := render(t, r, meterTable(TableCell{Text: "40 %", Fill: 40, Variant: variant}))
		if !strings.Contains(got, want+`"`) && !strings.Contains(got, want+" ") {
			t.Errorf("variant %q: want %s:\n%s", variant, want, got)
		}
	}
}

// TestMeterCellWithNoReadingIsADash: a thing that is off has no share to show,
// and an empty track would read as "idle" — the cell says nothing instead.
func TestMeterCellWithNoReadingIsADash(t *testing.T) {
	r := newRenderer(t)
	for _, cell := range []TableCell{{}, {Text: "—"}} {
		got := render(t, r, meterTable(cell))
		if strings.Contains(got, "clip-path") || strings.Contains(got, "bg-rule") {
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
		`bg-marigold" style="left: min(89%, calc(100% - 0.375rem))"`, // the warn band's own hue, as its bar fills past it
		`bg-crimson" style="left: calc(100% - 0.125rem)"`,
		`>48.0 °C<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
	// The ticks stand a pixel past the track above and below, so the track no
	// longer clips what it holds; the fill clips itself.
	if strings.Contains(got, "overflow-hidden rounded-full bg-rule") {
		t.Errorf("the track clips its ticks:\n%s", got)
	}

	// A warn limit at the protect limit stands just before its tick, 2px
	// apart, so every row reads marigold, then crimson; a warn limit with no
	// protect limit after it ends on the track's end, as a protect tick does.
	same := render(t, newRenderer(t), meterTable(TableCell{Text: "70.0 °C", Fill: 80, WarnMark: 100, CritMark: 100}))
	if !strings.Contains(same, `bg-marigold" style="left: min(100%, calc(100% - 0.375rem))"`) {
		t.Errorf("a warn limit at the protect limit should stand before it:\n%s", same)
	}
	alone := render(t, newRenderer(t), meterTable(TableCell{Text: "70.0 °C", Fill: 80, WarnMark: 100}))
	if !strings.Contains(alone, `bg-marigold" style="left: min(100%, calc(100% - 0.125rem))"`) {
		t.Errorf("a lone warn limit should end on the track's end:\n%s", alone)
	}
}

// TestMeterCellWithoutMarksIsUnchanged: a share (airtime, leases) has no
// limits, so its cell draws no tick.
func TestMeterCellWithoutMarksIsUnchanged(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "61 %", Fill: 61}))
	if strings.Contains(got, "-top-1 -bottom-1") {
		t.Errorf("a meter cell with no marks drew a tick:\n%s", got)
	}
}

// TestMeterCellWithNoTrackKeepsItsFigure: a reading with no limits has no
// honest headroom, so it shows its figure in the figure's slot and no track,
// never a bar against a guessed ceiling.
func TestMeterCellWithNoTrackKeepsItsFigure(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "71.0 °C", NoTrack: true}))
	if strings.Contains(got, "clip-path") || strings.Contains(got, "bg-rule") {
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
	if !strings.Contains(got, `w-16 shrink-0 text-right tabular-nums whitespace-nowrap`) {
		t.Errorf("the figure's slot should be 4rem, one line:\n%s", got)
	}
	// The track keeps a floor, so a narrow window scrolls the table rather
	// than shrinking the bar to a stub.
	if !strings.Contains(got, `min-w-16 flex-1`) {
		t.Errorf("the track should keep a minimum width:\n%s", got)
	}
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

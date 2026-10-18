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
// a row's scale — ten blocks lit to the cell's share, and the figure beside
// it — so a listing can say how full each of its things is.
func TestMeterCellDrawsTheShareAndItsFigure(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "61 %", Fill: 61, Variant: "warning"}))
	for _, want := range []string{
		`<span class="verso-meter" data-verso-place="cell">`,
		`<span class="verso-meter__track" style="--verso-meter-fill: 61">`, // the widget's own track, lit to 61%
		meterBar("warning"), // the plugin's band, in the badge vocabulary
		`>61 %<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
	declares(t, "meter", ".verso-meter", "--verso-meter-blocks: 10;")
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
		if strings.Contains(got, "verso-meter__track") {
			t.Errorf("a cell with no reading drew a track:\n%s", got)
		}
		if !strings.Contains(got, `text-faint"><span aria-hidden="true">—</span>`) {
			t.Errorf("a cell with no reading should be the faint dash:\n%s", got)
		}
	}
}

// TestMeterCellShadesItsWarnZone: a headroom listing (a temperature against
// the point the hardware protects itself) shades the blocks past its warn
// limit, and the track's end is the protect limit, so the bar says how far the
// reading is from each without a tick.
func TestMeterCellShadesItsWarnZone(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "48.0 °C", Fill: 50, WarnMark: 89}))
	for _, want := range []string{
		`<span class="verso-meter__track" style="--verso-meter-fill: 50; --verso-meter-warn: 89">`,
		`>48.0 °C<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter cell missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verso-meter__tick") {
		t.Errorf("the warn zone says the limit; the cell draws no tick:\n%s", got)
	}
}

// TestMeterCellWithoutMarksIsUnchanged: a share (airtime, leases) has no
// limits, so its cell shades no zone.
func TestMeterCellWithoutMarksIsUnchanged(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "61 %", Fill: 61}))
	if strings.Contains(got, "--verso-meter-warn") {
		t.Errorf("a meter cell with no limits shaded a zone:\n%s", got)
	}
}

// TestMeterCellWithNoTrackKeepsItsFigure: a reading with no limits has no
// honest headroom, so it shows its figure in the figure's slot and no track,
// never a bar against a guessed ceiling. The track's room stays, so its
// figure stands under the figures of the rows with a track.
func TestMeterCellWithNoTrackKeepsItsFigure(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "71.0 °C", NoTrack: true}))
	if strings.Contains(got, "verso-meter__track") {
		t.Errorf("a no-track cell drew a track:\n%s", got)
	}
	if !strings.Contains(got, `<span class="verso-meter__room"></span>`) {
		t.Errorf("a no-track cell does not keep the track's room:\n%s", got)
	}
	if !strings.Contains(got, `>71.0 °C<`) || strings.Contains(got, `aria-hidden="true">—<`) {
		t.Errorf("a no-track cell should show its figure, not the dash:\n%s", got)
	}
	declares(t, "meter", ".verso-meter__room", "flex: none;", "width: calc(var(--verso-meter-blocks) * var(--verso-meter-step) - var(--verso-meter-gap));")
}

// TestMeterCellFigureHoldsAReadingWithItsUnit: the figure's slot is one width
// for every row, 12px after the track, and it holds a temperature with its
// unit on one line.
func TestMeterCellFigureHoldsAReadingWithItsUnit(t *testing.T) {
	got := render(t, newRenderer(t), meterTable(TableCell{Text: "100.0 °C", Fill: 100}))
	if !strings.Contains(got, `class="verso-meter__amount `) {
		t.Errorf("the figure's slot should be 4rem, one line:\n%s", got)
	}
	declares(t, "meter", ".verso-meter__amount", "flex-shrink: 0;", "width: calc(var(--spacing) * 16);", "text-align: right;", "white-space: nowrap;", "font-variant-numeric: tabular-nums;")
	declares(t, "meter", `.verso-meter[data-verso-place="cell"]`, "gap: calc(var(--spacing) * 3);")
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

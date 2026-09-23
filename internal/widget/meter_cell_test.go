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
		if !strings.Contains(got, "text-inert") {
			t.Errorf("a cell with no reading should be the faint dash:\n%s", got)
		}
	}
}

// TestTextCellCarriesItsChip: a plain word can cite the config value it stands
// for — "WPA3" beside the `sae` it writes — in the same chip a name cell uses.
func TestTextCellCarriesItsChip(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Security"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "WPA3", Chip: "sae"}}}},
	})
	for _, want := range []string{">WPA3<", chipMonoBox + " border-rule bg-quiet text-meta", ">sae<"} {
		if !strings.Contains(got, want) {
			t.Errorf("text cell missing %q:\n%s", want, got)
		}
	}
}

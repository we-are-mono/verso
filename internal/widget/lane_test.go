// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func lanes() *Table {
	return &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{
			{ID: "a", Group: &TableGroup{Key: "input_wan", Label: "WAN", To: "Router", Tally: "6 rules",
				AddText: "Add rule", AddLabel: "Add rule to WAN → Router", AddHref: "/r?open=new&src=wan"}, Cells: []TableCell{{Text: "Allow-Ping"}}},
			{ID: "b", Cells: []TableCell{{Text: "Allow-IGMP"}}},
			{ID: "c", Group: &TableGroup{Key: "forward_wan", Label: "WAN", To: "Forwarded traffic", Tally: "3 rules"}, Cells: []TableCell{{Text: "Allow-ESP"}}},
		},
	}
}

// TestLaneIsARowNotABand: a group's head is a row of the listing's own height
// — no fill, a strong hairline under it — so the only filled surface over a
// listing is the band it is narrowed from.
func TestLaneIsARowNotABand(t *testing.T) {
	got := render(t, newRenderer(t), lanes())
	if strings.Contains(got, `verso-table-group h-13 bg-quiet`) {
		t.Errorf("a lane is a row, not a filled band:\n%s", got)
	}
	for _, want := range []string{
		`class="verso-table-group"`,
		`border-b border-rule-strong pt-1.5 pb-1.25 text-left leading-7`,  // the first lane: a row of two cells
		`border-b border-rule-strong pt-11.5 pb-1.25 text-left leading-7`, // a later one: two cells of air above
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lane rows missing %q:\n%s", want, got)
		}
	}
}

// TestLaneAddSaysWhatItDoes: a lane's add is a quiet labelled button — its
// words say the act, the tooltip says which chain it adds to — set in the meta
// ink (the glyph step is never text) and washed on hover.
func TestLaneAddSaysWhatItDoes(t *testing.T) {
	got := render(t, newRenderer(t), lanes())
	for _, want := range []string{
		`aria-label="Add rule to WAN → Router"`,
		`>Add rule</span>`,
		"h-7", "text-meta", "hover:text-ink",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lane add missing %q:\n%s", want, got)
		}
	}
	// Words that already name the act take no tooltip repeating them.
	same := lanes()
	same.Rows[0].Group.AddText, same.Rows[0].Group.AddLabel = "New file", "New file"
	if out := render(t, newRenderer(t), same); strings.Count(out, "New file") != 2 { // aria-label + words
		t.Errorf("a lane add whose words are its label draws no tooltip:\n%s", out)
	}
	// A lane that gives no words keeps the glyph alone.
	bare := lanes()
	bare.Rows[0].Group.AddText = ""
	if out := render(t, newRenderer(t), bare); strings.Contains(out, `>Add rule</span>`) {
		t.Errorf("a lane with no add words draws the glyph only:\n%s", out)
	}
}

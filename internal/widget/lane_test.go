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
			{ID: "a", Group: &TableGroup{Key: "input_wan", Label: "WAN", To: "Router",
				AddText: "Add rule", AddLabel: "Add rule to WAN → Router", AddHref: "/r?open=new&src=wan"}, Cells: []TableCell{{Text: "Allow-Ping"}}},
			{ID: "b", Cells: []TableCell{{Text: "Allow-IGMP"}}},
			{ID: "c", Group: &TableGroup{Key: "forward_wan", Label: "WAN", To: "Forwarded traffic"}, Cells: []TableCell{{Text: "Allow-ESP"}}},
		},
	}
}

// inSection is a listing that is part of a section, not a page's own block.
func inSection(w Widget) Widget { return &Section{Title: "Rules", Children: []Widget{w}} }

// TestAPagesLanesShareOneCard: a laned listing that is the page's own block is
// one card holding one table: the column heads once, then every lane as a
// band row before its rules, each band straight after the row above it with
// no air, since the card already sets the listing apart.
func TestAPagesLanesShareOneCard(t *testing.T) {
	got := render(t, newRenderer(t), &Stack{Children: []Widget{lanes()}})
	if n := strings.Count(got, "<table"); n != 1 {
		t.Fatalf("want one table for every lane, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, ">Name</th>"); n != 1 {
		t.Errorf("the column heads stand once, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, `class="verso-table-group"`); n != 2 {
		t.Errorf("want a band row per lane, got %d:\n%s", n, got)
	}
	if strings.Contains(got, "pt-11.25") {
		t.Errorf("a later lane in a page's card opens with air above its band:\n%s", got)
	}
	// Inside a section a later lane keeps the air above its band.
	if inner := render(t, newRenderer(t), inSection(lanes())); !strings.Contains(inner, "pt-11.25") {
		t.Errorf("a section's later lane lost its air:\n%s", inner)
	}
}

// TestLaneIsARowNotABand: a group's head is a row of the listing's own height
// on the masthead's Quiet Sand between strong hairlines; the air above a
// later lane stays the page's ground.
func TestLaneIsARowNotABand(t *testing.T) {
	got := render(t, newRenderer(t), inSection(lanes()))
	if strings.Contains(got, `verso-table-group h-13 bg-quiet`) {
		t.Errorf("a lane is a row, not a band of its own height:\n%s", got)
	}
	for _, want := range []string{
		`class="verso-table-group"`,
		// the first lane: a row of two cells, its add centred on the middle
		// line and ending where the rows' acts do (7px in from the table's edge)
		`border-b border-rule-strong bg-quiet pt-1.25 pb-1.5 pr-1.75! text-left leading-7`,
		// a later one: two cells of air above, unfilled, then the strong
		// hairline the first lane takes from the column heads
		`border-b border-rule-strong bg-[linear-gradient(transparent_calc(2.5rem_-_1px),var(--color-rule-strong)_calc(2.5rem_-_1px)_2.5rem,var(--color-quiet)_2.5rem)] pt-11.25 pb-1.5 pr-1.75! text-left leading-7`,
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
		// The button block's act size in its quiet dress, glyph first.
		`aria-label="Add rule to WAN → Router" class="verso-button group/act relative" data-verso-size="act" data-verso-style="quiet" data-verso-glyph>`,
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

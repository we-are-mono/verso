// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestTableScrollerContainsItsHiddenWords: a cell's screen-reader words (the
// dash's "None") are absolutely placed, so the scroller that clips a wide table
// has to be their containing block — otherwise a word in a column scrolled out
// of view widens the whole page on a phone.
func TestTableScrollerContainsItsHiddenWords(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Airtime busy", Kind: "meter"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{}}}},
	})
	if !strings.Contains(got, `class="relative overflow-x-auto overflow-y-hidden`) {
		t.Errorf("the table's scroller must contain its absolutely placed words:\n%s", got)
	}
}

// TestTitledTableStandsOffLikeASection: a listing with its own band is a
// section of the page, so it stands off whatever sits above it by the gap the
// homepage leaves before its System band — not the 16px between a listing's
// own parts. Leading the page, it takes no gap.
func TestTitledTableStandsOffLikeASection(t *testing.T) {
	r := newRenderer(t)
	titled := render(t, r, &Table{Title: "Radios", Columns: []TableColumn{{Label: "Radio"}}})
	if !strings.Contains(titled, `class="relative mt-11 overflow-x-auto overflow-y-hidden first:mt-0`) {
		t.Errorf("a titled listing should open with the section gap:\n%s", titled)
	}
	plain := render(t, r, &Table{Columns: []TableColumn{{Label: "Network"}}})
	if strings.Contains(plain, "mt-11") {
		t.Errorf("a listing without a band keeps its place in the stack:\n%s", plain)
	}
}

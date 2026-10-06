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

// TestTableScrollerSaysThereIsMore: a table wider than its box (a phone) keeps
// its trailing columns — a row's act among them — out of sight, so the scroller
// wears the edge that fades while more waits to the right.
func TestTableScrollerSaysThereIsMore(t *testing.T) {
	got := render(t, newRenderer(t), &Table{Columns: []TableColumn{{Label: "Source"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "wan"}}}}})
	if !strings.Contains(got, "verso-scroll-edge") {
		t.Errorf("the table's scroller must mark what waits past its edge:\n%s", got)
	}
}

// TestPageSpacingIsThePagesNotTheBlocks: how far a listing stands from what
// sits above it is the page's rhythm (the stack's), not the listing's own —
// a titled listing carries no margin of its own, and the page stack names
// itself so the rhythm can find it.
func TestPageSpacingIsThePagesNotTheBlocks(t *testing.T) {
	r := newRenderer(t)
	titled := render(t, r, &Table{Title: "Radios", Columns: []TableColumn{{Label: "Radio"}}})
	if strings.Contains(titled, "mt-11") || strings.Contains(titled, "mt-10") {
		t.Errorf("a listing carries no page gap of its own:\n%s", titled)
	}
	stack := render(t, r, &Stack{Children: []Widget{&Text{Markdown: "a"}, &Text{Markdown: "b"}}})
	if !strings.Contains(stack, `class="verso-stack space-y-5"`) {
		t.Errorf("a plain stack names itself for the page rhythm:\n%s", stack)
	}
	// The band's only margins put its frame on the grid's lines, a pixel out
	// into the air above and the gutter; it adds no gap of its own.
	bar := render(t, r, &ActionBar{Tabs: []ActionTab{{Label: "All", Count: 1, Active: true}}})
	const open = `data-verso-actionbar class="`
	at := strings.Index(bar, open)
	if at < 0 || !strings.HasPrefix(bar[at+len(open):], "-mt-px -ml-px flex") {
		t.Errorf("a control band's frame stands on the grid's lines:\n%s", bar)
	} else {
		class := bar[at+len(open):]
		for _, c := range strings.Fields(class[:strings.Index(class, `"`)]) {
			if strings.HasPrefix(c, "mt-") || strings.HasPrefix(c, "mb-") || strings.HasPrefix(c, "my-") {
				t.Errorf("a control band carries no margin of its own; it sits on its listing (%s):\n%s", c, bar)
			}
		}
	}
}

// TestRuledSectionStandsFortyAboveAndBelowItsRule: a hairline between two of
// a page's subjects has the page's gap on both sides of it — 2.5rem above the
// rule, 2.5rem from the rule to the section's heading — and the section a page
// opens with draws neither rule nor gap.
func TestRuledSectionStandsFortyAboveAndBelowItsRule(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Title: "Time", Hairline: true, Children: []Widget{&Text{Markdown: "x"}}})
	// The numbers are sections.css's; the section says it is ruled.
	want := `<section data-verso-section="ruled" data-verso-ruled data-verso-section-divider>`
	if !strings.Contains(got, want) {
		t.Errorf("ruled section should carry %q:\n%s", want, got)
	}
}

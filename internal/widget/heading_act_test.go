// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func actOnly() *ActionBar {
	return &ActionBar{Action: &TableAction{Label: "Add network", Href: "/plugins/wireless/?open=new", Icon: "plus"}, OpensPanel: true}
}

// TestTakeHeadingActLiftsABarThatOnlyActs: a page with nothing to search or
// narrow has no control band at all — its one act sits on the heading line,
// and the listing under it stays where it was.
func TestTakeHeadingActLiftsABarThatOnlyActs(t *testing.T) {
	table := &Table{Columns: []TableColumn{{Label: "Network"}}}
	bar := actOnly()
	page := &Stack{Children: []Widget{bar, table}}

	got := TakeHeadingAct(page)
	if got == nil || !got.Heading || got.Action == nil || got.Action.Label != "Add network" || !got.OpensPanel {
		t.Fatalf("the act was not lifted whole: %+v", got)
	}
	if len(page.Children) != 1 || page.Children[0] != table {
		t.Errorf("a band with nothing left on it goes; the listing stays: %v", page.Children)
	}
}

// TestTakeHeadingActLiftsThePrimaryOffABandThatNarrows: every page's primary
// act lives on its heading line, so a band that narrows the listing gives its
// act up and keeps only the narrowing — the search, the cuts, the select, the
// live control. The act keeps what it opens.
func TestTakeHeadingActLiftsThePrimaryOffABandThatNarrows(t *testing.T) {
	blank := &RowDrawer{Title: "New rule", Open: true}
	band := &ActionBar{
		Filter: "Find a rule", Tabs: []ActionTab{{Label: "All"}},
		Action: &TableAction{Label: "Add rule", Href: "/x?open=new", Icon: "plus"}, OpensPanel: true, Drawer: blank,
	}
	page := &Stack{Children: []Widget{band, &Table{}}}

	got := TakeHeadingAct(page)
	if got == nil || got.Action == nil || got.Action.Label != "Add rule" || got.Drawer != blank || !got.OpensPanel || !got.Heading {
		t.Fatalf("the act and its panel should move to the heading line: %+v", got)
	}
	if got.Filter != "" || len(got.Tabs) > 0 {
		t.Errorf("the heading takes the act alone: %+v", got)
	}
	if len(page.Children) != 2 || page.Children[0] != band {
		t.Fatalf("the band stays at the head of its listing: %v", page.Children)
	}
	if band.Action != nil || band.Drawer != nil || band.OpensPanel {
		t.Errorf("the band keeps only its narrowing: %+v", band)
	}
	if band.Filter != "Find a rule" || len(band.Tabs) != 1 {
		t.Errorf("the band's narrowing is untouched: %+v", band)
	}
}

// TestTakeHeadingActLeavesAQuietActAndOtherShapes: a quiet act takes something
// away from the listing rather than making something, so it stays with the
// narrowing; and only a band leading the page's stack has a heading to go to.
func TestTakeHeadingActLeavesAQuietActAndOtherShapes(t *testing.T) {
	quiet := &ActionBar{Filter: "Find", Action: &TableAction{Label: "Download", Href: "/x", Style: "quiet"}}
	if got := TakeHeadingAct(&Stack{Children: []Widget{quiet, &Table{}}}); got != nil || quiet.Action == nil {
		t.Error("a quiet act stays on the band")
	}
	none := &ActionBar{Filter: "Find"}
	if got := TakeHeadingAct(&Stack{Children: []Widget{none, &Table{}}}); got != nil {
		t.Error("a band with no act gives nothing to the heading")
	}
	if got := TakeHeadingAct(&Stack{Children: []Widget{&Table{}, actOnly()}}); got != nil {
		t.Error("only a band leading the page stands for the heading's act")
	}
	if got := TakeHeadingAct(actOnly()); got != nil {
		t.Error("a band that is the whole page has no heading to move to")
	}
}

// TestHeadingActRendersTheActAlone: on the heading line the act is the 36px
// primary with no band around it.
func TestHeadingActRendersTheActAlone(t *testing.T) {
	bar := actOnly()
	bar.Heading = true
	got := render(t, newRenderer(t), bar)
	for _, want := range []string{`@click.prevent="showPanel"`, "h-9", "bg-denim", ">Add network<"} {
		if !strings.Contains(got, want) {
			t.Errorf("heading act missing %q:\n%s", want, got)
		}
	}
	for _, never := range []string{"data-verso-actionbar", "data-verso-listing-filter", "bg-quiet"} {
		if strings.Contains(got, never) {
			t.Errorf("heading act carries the band's %q:\n%s", never, got)
		}
	}
}

// TestControlBandIsTheListingsSurface: the band that narrows a listing is its
// one filled surface — the quiet sand between two hairlines, every control 16px
// from its edges — and it sits flush on the listing's column heads.
func TestControlBandIsTheListingsSurface(t *testing.T) {
	got := render(t, newRenderer(t), &ActionBar{Filter: "Find a rule"})
	// mb-0 outranks a nested stack's space-y, which would part band and table.
	if !strings.Contains(got, `data-verso-actionbar class="flex flex-wrap items-center gap-4 mb-0 border-y border-rule bg-quiet p-4"`) {
		t.Errorf("control band surface wrong:\n%s", got)
	}
	if strings.Contains(got, "mb-5") || strings.Contains(got, "mb-8") {
		t.Errorf("the band sits flush on its listing:\n%s", got)
	}
	// A live log's bar is the system log's: unfilled, over a strong hairline.
	live := render(t, newRenderer(t), &ActionBar{Filter: "Find an address", Live: "Pause"})
	if strings.Contains(live, "bg-quiet p-4") || !strings.Contains(live, `gap-4 mb-0 -mx-10 border-b border-rule-strong pr-11 pb-5 pl-10"`) {
		t.Errorf("a live log's bar is plain:\n%s", live)
	}
}

// TestColumnHeadsAreARow: the heads are the listing's own 44px row — a 24px
// line with 10px above and below — whatever introduces the table.
func TestColumnHeadsAreARow(t *testing.T) {
	for _, tbl := range []*Table{
		{Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "x"}}}}},
		{Title: "Rules", Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "x"}}}}},
	} {
		got := render(t, newRenderer(t), tbl)
		if !strings.Contains(got, "px-3.5 py-2.5 leading-6 text-xs") || strings.Contains(got, "pt-4 pb-2") {
			t.Errorf("column heads are not a 44px row:\n%s", got)
		}
	}
}

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

// TestDecodeHeadingAct: a page's act arrives beside the page, not inside it,
// and carries what it opens — a blank object's panel decodes through
// RowDrawer's own decoder, typed children and all.
func TestDecodeHeadingAct(t *testing.T) {
	act, err := DecodeHeadingAct([]byte(`{"label":"Add rule","href":"/x?open=new","opens_panel":true,
		"drawer":{"title":"New rule","open":true,"closed":"/x","children":[{"type":"text","markdown":"blank"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	bar := act.Bar()
	if !bar.Heading || bar.Action == nil || bar.Action.Label != "Add rule" || !bar.OpensPanel {
		t.Fatalf("the act should draw on the heading line whole: %+v", bar)
	}
	if bar.Drawer == nil || !bar.Drawer.Open || len(bar.Drawer.Children) != 1 {
		t.Fatalf("the act's blank panel should decode with its children: %+v", bar.Drawer)
	}
	if empty, err := DecodeHeadingAct(nil); err != nil || empty != nil {
		t.Errorf("no act is no act: %v, %v", empty, err)
	}
	if (*HeadingAct)(nil).Bar() != nil {
		t.Error("no act draws no heading")
	}
}

// TestTheActRidesWithThePageUntilItIsDrawn: between decoding and drawing, the
// shell's passes over a page (staging marks, refusals, the open panel, the
// frame a panel's forms post into) see the act's blank panel exactly as they
// see a row's, so the act travels in the tree; it leaves it only to be drawn on
// the heading line.
func TestTheActRidesWithThePageUntilItIsDrawn(t *testing.T) {
	page := &Stack{Children: []Widget{&Table{}}}
	act := &HeadingAct{Label: "Add rule", Href: "/x?open=new", Drawer: &RowDrawer{Title: "New rule", Open: true}}
	tree := WithHeading(act, page)
	if openPanel(tree) != act.Drawer {
		t.Error("the act's open blank panel should be found like a row's")
	}
	bar, rest := SplitHeading(tree)
	if bar == nil || !bar.Heading || bar.Drawer != act.Drawer || rest != page {
		t.Errorf("the act should come off whole and leave the page as it was: %+v, %v", bar, rest)
	}
	if bar, rest := SplitHeading(page); bar != nil || rest != page {
		t.Error("a page with no act gives none")
	}
	if WithHeading(nil, page) != page {
		t.Error("no act leaves the page alone")
	}
}

// TestABarTakesNoActFromAPlugin: a listing's bar narrows the listing and
// nothing else. A page's act is the envelope's own, so an act a plugin puts on
// a bar is not read.
func TestABarTakesNoActFromAPlugin(t *testing.T) {
	w, err := Decode([]byte(`{"type":"actionbar","live":"Live","filter":"Find",
		"action":{"label":"Add rule","href":"/x"},"opens_panel":true,
		"drawer":{"title":"New rule","open":true,"children":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	bar := w.(*ActionBar)
	if bar.Action != nil || bar.Drawer != nil || bar.OpensPanel {
		t.Errorf("the bar should take no act from a plugin: %+v", bar)
	}
	if bar.Live != "Live" || bar.Filter != "Find" {
		t.Errorf("the bar keeps what narrows: %+v", bar)
	}
}

// TestTakeLiveJoinsTheLogsActs: a live log has no band. Its search and its
// live control stand on the heading line beside the page's act, as equals,
// and the bar leaves the page. Only a bar leading the page's stack has a
// heading to give them to.
func TestTakeLiveJoinsTheLogsActs(t *testing.T) {
	page := &Stack{Children: []Widget{&ActionBar{Filter: "Find", Live: "Live"}, &Table{Style: "console"}}}
	live, filter := TakeLive(page)
	if live != "Live" || filter != "Find" || len(page.Children) != 1 {
		t.Fatalf("the live control and the search should leave the page for the heading: %q %q, page %+v", live, filter, page.Children)
	}
	if got, _ := TakeLive(&Stack{Children: []Widget{&Table{}, &ActionBar{Live: "Live"}}}); got != "" {
		t.Error("only a bar leading the page gives up its live control")
	}
	act := (&HeadingAct{Label: "Download", Href: "/x", Style: "quiet"}).Bar()
	act.Live, act.Filter = live, filter
	heading := render(t, newRenderer(t), act)
	search, pause, download := strings.Index(heading, "data-verso-listing-filter"), strings.Index(heading, "data-verso-live"), strings.Index(heading, ">Download<")
	if search < 0 || pause < search || download < pause {
		t.Errorf("the heading line should hold the search, Live, then Download:\n%s", heading)
	}
	// The search narrows the log from up here: the group is the log's bar.
	if !strings.Contains(heading, `<div data-verso-actionbar="log" class="verso-actionbar">`) {
		t.Errorf("the heading's log group is not the log's bar:\n%s", heading)
	}
	// As equals on the heading line: no band of its own around them.
	declares(t, "actionbar", ".verso-actionbar", "display: flex", "flex-wrap: wrap", "align-items: center")
	declares(t, "actionbar", `.verso-actionbar[data-verso-actionbar="log"]`,
		"gap: calc(var(--spacing) * 2)", "min-width: 0",
		"padding: 0", "border: 0", "background-color: transparent")
}

// TestPrimaryActsWearThePlus: every page's primary act wears the plus —
// whether it opens a panel, an entity's panel or a page, and whether or not
// the plugin named the glyph — unless it names one that leads elsewhere.
func TestPrimaryActsWearThePlus(t *testing.T) {
	plus := render(t, newRenderer(t), &ActionBar{Heading: true, Action: &TableAction{Label: "Plus", Href: "/p", Icon: "plus"}})
	if !strings.Contains(plus, "<svg") {
		t.Fatalf("a named plus should draw a glyph:\n%s", plus)
	}
	glyph := plus[strings.Index(plus, "<svg"):strings.Index(plus, "</svg>")]
	for name, bar := range map[string]*ActionBar{
		"panel":  {Heading: true, OpensPanel: true, Action: &TableAction{Label: "Add rule", Href: "/r?open=new"}},
		"entity": {Heading: true, Entity: "/entity/device/new", Action: &TableAction{Label: "Reserve address", Href: "/d"}},
		"page":   {Heading: true, Action: &TableAction{Label: "Install", Href: "/i"}},
	} {
		if got := render(t, newRenderer(t), bar); !strings.Contains(got, glyph) {
			t.Errorf("%s: a primary act should wear the plus:\n%s", name, got)
		}
	}
	if got := render(t, newRenderer(t), &ActionBar{Heading: true, Action: &TableAction{Label: "Get", Href: "/g", Icon: "download"}}); strings.Contains(got, glyph) {
		t.Errorf("an act that names its own glyph keeps it:\n%s", got)
	}
}

// TestHeadingActRendersTheActAlone: on the heading line the act is the 34px
// primary with no band around it.
func TestHeadingActRendersTheActAlone(t *testing.T) {
	bar := actOnly()
	bar.Heading = true
	got := render(t, newRenderer(t), bar)
	// The button block in no other dress or size is the 32px primary.
	for _, want := range []string{`@click.prevent="showPanel" class="verso-button shrink-0">`, ">Add network<"} {
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

// TestControlBandIsTheListingCardsTopStrip: the controls that narrow a
// listing are the top strip of its card: the card's Paper and frame
// over a hairline, its top border and its hairline on lines 64px apart (15px
// over the controls, 16px under), the controls 23px after its left border so
// the first stands on a line, no Mid Sand box; the listing under it closes
// the card.
func TestControlBandIsTheListingCardsTopStrip(t *testing.T) {
	got := render(t, newRenderer(t), &ActionBar{Tabs: []ActionTab{{Label: "All families", Count: 3, Active: true}}})
	if !strings.Contains(got, `<div data-verso-actionbar class="verso-actionbar">`) {
		t.Errorf("control band surface wrong:\n%s", got)
	}
	declares(t, "actionbar", ".verso-actionbar",
		"display: flex", "flex-wrap: wrap", "align-items: center", "gap: calc(var(--spacing) * 4)",
		"padding: calc(var(--spacing) * 3.75) calc(var(--spacing) * 5.75) calc(var(--spacing) * 4)",
		"border: 1px solid var(--color-rule-strong)", "border-bottom-color: var(--color-rule)",
		"border-radius: var(--radius-xs) var(--radius-xs) 0 0", "background-color: var(--color-ground)")
	if strings.Contains(got, "mb-5") || strings.Contains(got, "mb-8") {
		t.Errorf("the band sits flush on its listing:\n%s", got)
	}
	console := render(t, newRenderer(t), &Table{Style: "console", Stream: &TableStream{Source: StreamSourceFirewallLog}})
	if !strings.Contains(console, `<div class="verso-console" data-verso-kind="events"`) {
		t.Errorf("a live log is the console block:\n%s", console)
	}
	declares(t, "console", ".verso-console",
		"margin-inline: calc(var(--spacing) * -10)", "background-color: var(--color-quiet)", "box-shadow: var(--shadow-edge)")
}

// TestColumnHeadsAreARow: the heads are one row of the grid — the 12px caps
// on a 16px line, 4px over it and 3px under it plus the 1px hairline: 24px —
// whatever introduces the table. The table block draws the head; the markup
// only names it.
func TestColumnHeadsAreARow(t *testing.T) {
	for _, tbl := range []*Table{
		{Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "x"}}}}},
		{Title: "Rules", Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "x"}}}}},
	} {
		got := render(t, newRenderer(t), tbl)
		if !strings.Contains(got, `<th scope="col" class="verso-table__head">Name</th>`) || strings.Contains(got, "pt-4 pb-2") {
			t.Errorf("column heads are not a row of the grid:\n%s", got)
		}
	}
	declares(t, "table", ".verso-table__head",
		"padding: var(--spacing) calc(var(--spacing) * 3.5) calc(var(--spacing) * 0.75)",
		"border-bottom-width: 1px", "@apply type-label")
}

// TestAQuietHeadingActOpensItsPanelInTheSecondaryDress: an act that opens a
// panel of settings over the page, not the thing the page is for, wears the
// quiet button, as Logs' Settings does.
func TestAQuietHeadingActOpensItsPanelInTheSecondaryDress(t *testing.T) {
	got := render(t, newRenderer(t), &ActionBar{Heading: true, OpensPanel: true,
		Action: &TableAction{Label: "Alerts", Href: "/system/events/alerts", Icon: "bell", Style: "quiet"}})
	if !strings.Contains(got, `@click.prevent="showPanel" class="verso-button shrink-0" data-verso-style="secondary">`) || strings.Contains(got, `class="verso-button shrink-0">`) {
		t.Errorf("a quiet act that opens a panel is not in the secondary dress:\n%s", got)
	}
}

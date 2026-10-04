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

// TestTakeLiveJoinsTheLogsActs: a live log's live control stands on the
// heading line beside the page's act, as its equal; the bar keeps what narrows
// the log and still wears the log's unfilled dress. Only a bar leading the
// page's stack has a heading to give it to.
func TestTakeLiveJoinsTheLogsActs(t *testing.T) {
	bar := &ActionBar{Filter: "Find", Tabs: []ActionTab{{Label: "All traffic", Active: true}}, Live: "Live"}
	lifted := TakeLive(&Stack{Children: []Widget{bar, &Table{Style: "console"}}})
	if lifted != "Live" || bar.Live != "" {
		t.Fatalf("the live control should leave the bar for the heading: %q, bar %+v", lifted, bar)
	}
	if got := TakeLive(&Stack{Children: []Widget{&Table{}, &ActionBar{Live: "Live"}}}); got != "" {
		t.Error("only a bar leading the page gives up its live control")
	}
	act := (&HeadingAct{Label: "Download", Href: "/x", Style: "quiet"}).Bar()
	act.Live = lifted
	heading := render(t, newRenderer(t), act)
	live, download := strings.Index(heading, "data-verso-live"), strings.Index(heading, ">Download<")
	if live < 0 || download < 0 || live > download || strings.Contains(heading, "<svg") {
		t.Errorf("the heading line should hold Live then Download, words alone:\n%s", heading)
	}
	left := render(t, newRenderer(t), bar)
	if !strings.Contains(left, `data-verso-actionbar class="mb-0 -mx-10 border-y border-rule bg-quiet px-10 py-4"`) || strings.Contains(left, "data-verso-live") {
		t.Errorf("the bar keeps the log's dress and only its narrowing:\n%s", left)
	}
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
		"entity": {Heading: true, Entity: "/entity/device/new", Action: &TableAction{Label: "Reserve an address", Href: "/d"}},
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
	got := render(t, newRenderer(t), &ActionBar{Tabs: []ActionTab{{Label: "All families", Count: 3, Active: true}}})
	// mb-0 outranks a nested stack's space-y, which would part band and table.
	if !strings.Contains(got, `data-verso-actionbar class="flex flex-wrap items-center gap-4 mb-0 border-y border-rule bg-quiet p-4"`) {
		t.Errorf("control band surface wrong:\n%s", got)
	}
	if strings.Contains(got, "mb-5") || strings.Contains(got, "mb-8") {
		t.Errorf("the band sits flush on its listing:\n%s", got)
	}
	// A live log's bar is the system log's: the same sand band run to the
	// window's edges, its controls kept to the content column.
	live := render(t, newRenderer(t), &ActionBar{Filter: "Find an address", Live: "Live"})
	for _, want := range []string{
		`data-verso-actionbar class="mb-0 -mx-10 border-y border-rule bg-quiet px-10 py-4"`,
		`<div class="flex w-full max-w-6xl flex-wrap items-center gap-4">`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("a live log's bar missing %q:\n%s", want, live)
		}
	}
	if strings.Contains(live, "bg-quiet p-4") || !strings.Contains(live, "data-verso-wait") {
		t.Errorf("a live log's band runs edge to edge, and its live control carries the spinner:\n%s", live)
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

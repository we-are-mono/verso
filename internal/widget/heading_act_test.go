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
// narrow has no toolbar — its one act sits on the heading line (the canvas's
// list-page rule). A bar that declares only its act is lifted out of the body
// for the shell to place there; the listing under it stays where it was.
func TestTakeHeadingActLiftsABarThatOnlyActs(t *testing.T) {
	table := &Table{Columns: []TableColumn{{Label: "Network"}}}
	bar := actOnly()
	page := &Stack{Children: []Widget{bar, table}}

	got := TakeHeadingAct(page)
	if got != bar {
		t.Fatalf("the bar that only acts was not lifted: %v", got)
	}
	if !got.Heading {
		t.Error("the lifted bar should render as the heading's act")
	}
	if len(page.Children) != 1 || page.Children[0] != table {
		t.Errorf("the listing should stay the body, alone: %v", page.Children)
	}
}

// TestTakeHeadingActLeavesABarThatNarrows: a bar with anything to narrow by is
// the listing's toolbar, and its act stays on it.
func TestTakeHeadingActLeavesABarThatNarrows(t *testing.T) {
	for name, bar := range map[string]*ActionBar{
		"filter": {Filter: "Find a zone", Action: &TableAction{Label: "Add zone", Href: "/x"}},
		"tabs":   {Tabs: []ActionTab{{Label: "All"}}, Action: &TableAction{Label: "Add rule", Href: "/x"}},
		"select": {Select: &ActionPick{Key: "network"}, Action: &TableAction{Label: "Add", Href: "/x"}},
		"live":   {Live: "Pause", Action: &TableAction{Label: "Add", Href: "/x"}},
		"no act": {},
	} {
		page := &Stack{Children: []Widget{bar, &Table{}}}
		if got := TakeHeadingAct(page); got != nil {
			t.Errorf("%s: a toolbar was lifted", name)
		}
		if len(page.Children) != 2 {
			t.Errorf("%s: the body lost a child", name)
		}
	}
	if got := TakeHeadingAct(&Stack{Children: []Widget{&Table{}, actOnly()}}); got != nil {
		t.Error("only a bar leading the page stands for the heading's act")
	}
	if got := TakeHeadingAct(actOnly()); got != nil {
		t.Error("a bar that is the whole page has no heading to move to")
	}
}

// TestHeadingActRendersTheActAlone: on the heading line the act is the 36px
// primary with no bar around it — no search, no 32px gap to a listing.
func TestHeadingActRendersTheActAlone(t *testing.T) {
	bar := actOnly()
	bar.Heading = true
	got := render(t, newRenderer(t), bar)
	for _, want := range []string{`@click.prevent="showPanel"`, "h-9", "bg-denim", ">Add network<"} {
		if !strings.Contains(got, want) {
			t.Errorf("heading act missing %q:\n%s", want, got)
		}
	}
	for _, never := range []string{"data-verso-actionbar", "data-verso-listing-filter", "mb-8"} {
		if strings.Contains(got, never) {
			t.Errorf("heading act carries the bar's %q:\n%s", never, got)
		}
	}
}

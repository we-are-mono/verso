// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestActionBarCutsStandTogether: every cut reads left to right from the
// search — the counted dropdown, then the dimension the listing is sliced
// along — so no dropdown strays to the far end of the band, which holds only
// what acts.
func TestActionBarCutsStandTogether(t *testing.T) {
	got := render(t, newRenderer(t), &ActionBar{
		Filter: "Filter · name, address, MAC",
		Tabs:   []ActionTab{{Label: "All devices", Count: 14, Active: true}, {Label: "Online", Match: "online", Count: 1}},
		Select: &ActionPick{Key: "network", Options: []ActionOption{{Label: "All networks"}}},
		Action: &TableAction{Label: "Clear", Href: "/x", Style: "quiet"},
	})
	cut := strings.Index(got, "data-verso-listing-cut")
	pick := strings.Index(got, `data-verso-listing-select="network"`)
	right := strings.Index(got, "ml-auto")
	if cut < 0 || pick < 0 || right < 0 {
		t.Fatalf("bar lost a part (cut %d, select %d, acts %d):\n%s", cut, pick, right, got)
	}
	if !(cut < pick && pick < right) {
		t.Errorf("the select stands beside the counted dropdown, before the acts (cut %d, select %d, acts %d):\n%s", cut, pick, right, got)
	}
}

// TestActionBarSearchesOnlyALiveListing: a listing that holds what it has is
// read by scrolling and found in with the browser's own find, so its bar draws
// no search field, whatever the plugin asked for; the cuts stay, since they
// say what the listing holds. Over a live listing the field stays: its rows
// arrive while it is read, and the browser's find cannot hold a question
// across them.
func TestActionBarSearchesOnlyALiveListing(t *testing.T) {
	still := render(t, newRenderer(t), &ActionBar{
		Filter: "Find a rule",
		Tabs:   []ActionTab{{Label: "All families", Count: 3, Active: true}, {Label: "IPv4", Match: "ipv4", Count: 2}},
	})
	if strings.Contains(still, "data-verso-listing-filter") {
		t.Errorf("a still listing's bar should draw no search field:\n%s", still)
	}
	if !strings.Contains(still, "data-verso-listing-cut") {
		t.Errorf("the cuts should stay:\n%s", still)
	}
	live := render(t, newRenderer(t), &ActionBar{Filter: "Find an address, port or rule", Live: "Live"})
	if !strings.Contains(live, `data-verso-listing-filter autocomplete="off" placeholder="Find an address, port or rule"`) {
		t.Errorf("a live listing's bar should keep its search field:\n%s", live)
	}
}

// TestABandWithNothingLeftGoes: a still listing's bar that held only a search
// has nothing left to narrow with, so it draws nothing rather than standing
// empty.
func TestABandWithNothingLeftGoes(t *testing.T) {
	if got := render(t, newRenderer(t), &ActionBar{Filter: "Find a zone"}); strings.Contains(got, "data-verso-actionbar") {
		t.Errorf("a bar with nothing to show should draw nothing:\n%s", got)
	}
}

// TestHeadingActCarriesItsPanel: a page's act can open a blank object's panel,
// and the panel decodes through RowDrawer's own decoder, tabs and all. It is
// the same decoder a row's panel uses on purpose — when the act lived inside a
// row's decoder, its panel decoded to an error nobody saw and the act silently
// fell back to being a link.
func TestHeadingActCarriesItsPanel(t *testing.T) {
	act, err := DecodeHeadingAct([]byte(`{"label":"Add rule","href":"/x?open=new",
		"drawer":{"title":"New rule","open":true,"closed":"/x","lede":["Added to the end."],
			"tabs":[{"label":"Match","active":true}],
			"children":[{"type":"form","submit":"Add rule","fields":[{"type":"field","name":"n","label":"Name"}]}]}}`))
	if err != nil {
		t.Fatalf("DecodeHeadingAct: %v", err)
	}
	if act.Drawer == nil {
		t.Fatal("the act's panel was dropped in decode")
	}
	if act.Drawer.Title != "New rule" || !act.Drawer.Open || len(act.Drawer.Children) != 1 {
		t.Errorf("panel lost in decode: %#v", act.Drawer)
	}
	if len(act.Drawer.Tabs) != 1 || act.Drawer.Tabs[0].Label != "Match" {
		t.Errorf("panel tabs lost in decode: %#v", act.Drawer.Tabs)
	}
	// And it renders: the act hosts the modal scope, arrives open, and carries
	// the panel — while the anchor underneath stays the scriptless fallback.
	got := render(t, newRenderer(t), act.Bar())
	for _, want := range []string{
		// Opened by address, so closing it has to leave that address: the scope
		// carries where the page goes when nothing is open.
		`<span x-data="modal" data-open="true" data-closed-href="/x" class="contents">`,
		`<a href="/x?open=new" @click.prevent="show"`,
		"x-teleport", "New rule", "bg-quiet px-10", `aria-label="Sections"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("action bar panel missing %q:\n%s", want, got)
		}
	}
}

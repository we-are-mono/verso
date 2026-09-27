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

// TestDecodeActionBarDrawer: the bar's act can open a panel, and the panel
// decodes through RowDrawer's own decoder. It is the same decoder a row's panel
// uses on purpose — when this lived inside the row's, the bar's panel decoded to
// an error nobody saw and the act silently fell back to being a link.
func TestDecodeActionBarDrawer(t *testing.T) {
	w, err := Decode([]byte(`{"type":"actionbar","filter":"Find a rule",
		"action":{"label":"Add rule","href":"/x?open=new"},
		"drawer":{"title":"New rule","open":true,"closed":"/x","lede":["Added to the end."],
			"tabs":[{"label":"Match","state":"0 conditions","active":true}],
			"children":[{"type":"form","submit":"Add rule","fields":[{"type":"field","name":"n","label":"Name"}]}]}}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	bar := w.(*ActionBar)
	if bar.Drawer == nil {
		t.Fatal("the bar's panel was dropped in decode")
	}
	if bar.Drawer.Title != "New rule" || !bar.Drawer.Open || len(bar.Drawer.Children) != 1 {
		t.Errorf("panel lost in decode: %#v", bar.Drawer)
	}
	if len(bar.Drawer.Tabs) != 1 || bar.Drawer.Tabs[0].State != "0 conditions" {
		t.Errorf("panel tabs lost in decode: %#v", bar.Drawer.Tabs)
	}
	// And it renders: the act hosts the modal scope, arrives open, and carries
	// the panel — while the anchor underneath stays the scriptless fallback.
	got := render(t, newRenderer(t), w)
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestDecodeActionBarDrawer: the bar's act can open a panel, and the panel
// decodes through RowDrawer's own decoder. It is the same decoder a row's panel
// uses on purpose — when this lived inside the row's, the bar's panel decoded to
// an error nobody saw and the act silently fell back to being a link.
func TestDecodeActionBarDrawer(t *testing.T) {
	w, err := Decode([]byte(`{"type":"actionbar","filter":"Find a rule",
		"action":{"label":"Add rule","href":"/x?open=new"},
		"drawer":{"title":"New rule","open":true,"size":"wide","closed":"/x","lede":["Added to the end."],
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
		"x-teleport", "New rule", "bg-quiet px-8", `aria-label="Sections"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("action bar panel missing %q:\n%s", want, got)
		}
	}
}

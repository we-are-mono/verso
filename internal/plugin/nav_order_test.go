// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"strings"
	"testing"
)

func navigator(id, nav string) string {
	return `{"id":"` + id + `","name":"N","socket":"/run/verso/` + id + `.sock","schema_version":1,"nav":` + nav + `}`
}

// TestANavEntryDeclaresItsPlace: an entry's order is its place among its
// siblings, present or absent; a place below 1 reads as 1, after Overview's 0.
func TestANavEntryDeclaresItsPlace(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/vpn/manifest.json": navigator("vpn", `[
			{"section":"VPN","label":"VPN","path":"/","order":40},
			{"section":"VPN","label":"Profiles","path":"/profiles"},
			{"section":"VPN","label":"Top","path":"/top","order":-5}
		]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
	nav := got[0].Nav
	if nav[0].Order == nil || *nav[0].Order != 40 {
		t.Errorf("VPN order = %v, want 40", nav[0].Order)
	}
	if nav[1].Order != nil {
		t.Errorf("Profiles order = %v, want none", *nav[1].Order)
	}
	if nav[2].Order == nil || *nav[2].Order != 1 {
		t.Errorf("Top order = %v, want -5 read as 1", nav[2].Order)
	}
}

// TestAPlaceThatIsNotAWholeNumberIsNoPlace: text or a fraction is logged and
// read as no place, and the plugin and its other entries load.
func TestAPlaceThatIsNotAWholeNumberIsNoPlace(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/vpn/manifest.json": navigator("vpn", `[
			{"section":"VPN","label":"VPN","path":"/","order":"40"},
			{"section":"VPN","label":"Profiles","path":"/profiles","order":4.5},
			{"section":"VPN","label":"Status","path":"/status","order":30}
		]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 {
		t.Fatalf("the plugin did not load: %v", problems)
	}
	nav := got[0].Nav
	if nav[0].Order != nil || nav[1].Order != nil {
		t.Errorf("malformed places read as %v and %v, want none", nav[0].Order, nav[1].Order)
	}
	if nav[2].Order == nil || *nav[2].Order != 30 {
		t.Errorf("a good place beside them = %v, want 30", nav[2].Order)
	}
	said := ""
	for _, p := range problems {
		said += p.Error() + "\n"
	}
	for _, want := range []string{"nav[0]", "nav[1]", "vpn"} {
		if !strings.Contains(said, want) {
			t.Errorf("problems do not name %s: %s", want, said)
		}
	}
}

// TestAManifestWithNoPlacesLoadsAsBefore: order is optional.
func TestAManifestWithNoPlacesLoadsAsBefore(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/old/manifest.json": navigator("old", `[{"section":"Network","label":"Old","path":"/"}]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 || got[0].Nav[0].Order != nil {
		t.Errorf("got=%v problems=%v", got, problems)
	}
}

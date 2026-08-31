// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderOverviewPreview: the workbench renders the home masthead — verdict,
// view switch, tile strip, connection facts — from authored data, through the
// same define the real page uses, with no live hooks.
func TestRenderOverviewPreview(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &OverviewPreview{
		Kicker: "ALL GOOD", Lead: "Your network is ", Accent: "healthy",
		Tiles: []OverviewPreviewTile{
			{Label: "INTERNET", Icon: "globe", Variant: "success", Status: "Connected", Caption: "for 2h 14m"},
			{Label: "WI-FI", Icon: "wifi", Variant: "success", Status: "Both bands active", Caption: "2.4 & 5 GHz"},
			{Label: "SECURITY", Icon: "shield", Variant: "success", Status: "Protected", Caption: "Firewall on"},
			{Label: "SOFTWARE", Icon: "download", Variant: "warning", Status: "Update available", Caption: "Security fixes"},
		},
		Facts: []OverviewPreviewFactCol{
			{Kind: "IPV4", Proto: "DHCP", Facts: []OverviewPreviewFact{{Label: "Address", Value: "172.30.1.171/24", Copy: true}}},
			{Kind: "IPV6", Proto: "DHCPv6 client", Facts: []OverviewPreviewFact{{Label: "Prefix", Value: "fd42:7ea:aa00::/56"}}},
		},
	})
	for _, want := range []string{
		"ALL GOOD", "healthy", "font-serif", // the verdict sentence
		"Basic", "Advanced", // the view switch
		"grid grid-cols-4", // four tiles, four columns
		"INTERNET", "Connected", "for 2h 14m", "text-amber-700", // tiles, warning word tinted
		"IPV4", "DHCP", `font-mono font-semibold">172.30.1.171/24`, `x-data="copy"`, // facts + copy control
		"IPV6", "fd42:7ea:aa00::/56",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview preview missing %q in: %s", want, got)
		}
	}
	// A playground block is inert: no live caption hook, no stream targets.
	if strings.Contains(got, "data-verso-tile-caption") {
		t.Errorf("the preview must carry no live hooks: %s", got)
	}

	// Three tiles collapse to three full-width columns.
	three := render(t, r, &OverviewPreview{Tiles: []OverviewPreviewTile{{Label: "A"}, {Label: "B"}, {Label: "C"}}})
	if !strings.Contains(three, "grid grid-cols-3") {
		t.Errorf("three tiles should render three columns: %s", three)
	}
}

// TestDecodeOverviewPreview: the wire shape round-trips through Decode.
func TestDecodeOverviewPreview(t *testing.T) {
	w, err := Decode([]byte(`{"type":"overview-preview","kicker":"ALL GOOD","lead":"Your network is ","accent":"healthy",
		"tiles":[{"label":"INTERNET","icon":"globe","variant":"success","status":"Connected","caption":"for 2h 14m"}],
		"facts":[{"kind":"IPV4","proto":"DHCP","facts":[{"label":"Address","value":"172.30.1.171/24","copy":true}]}]}`))
	if err != nil {
		t.Fatalf("decode overview-preview: %v", err)
	}
	p, ok := w.(*OverviewPreview)
	if !ok {
		t.Fatalf("decoded %T, want *OverviewPreview", w)
	}
	if p.Accent != "healthy" || len(p.Tiles) != 1 || len(p.Facts) != 1 || !p.Facts[0].Facts[0].Copy {
		t.Errorf("overview-preview not decoded: %+v", p)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestStatHref: with an href the whole tile is a link (a doorway) with a chevron;
// without one it's a static div.
func TestStatHref(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stat{Label: "Internet", Value: "Online", Href: "/plugins/network/wan"})
	if !strings.Contains(got, `<a href="/plugins/network/wan"`) {
		t.Errorf("stat with href should be a link:\n%s", got)
	}
	if !strings.Contains(got, "m9 18 6-6-6-6") { // chevron-right — the doorway affordance
		t.Errorf("doorway stat should show a chevron:\n%s", got)
	}
	if strings.Contains(render(t, r, &Stat{Label: "Devices", Value: "9"}), "<a href") {
		t.Error("a stat without href should be a static div, not a link")
	}
}

func TestRenderStat(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stat{
		Label: "Devices", Value: "9", Sub: "online now", Icon: "devices", Variant: "good", Dot: true,
	})
	for _, want := range []string{
		"Devices", ">9", "online now",
		"rounded-xl",     // the radius-scale step for a tile
		"tabular-nums",   // figures align
		"verso-live-dot", // the pulsing dot when good
		"bg-green-500",   // dot tinted by the good variant
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stat missing %q:\n%s", want, got)
		}
	}
}

// TestRenderStatUnit renders the optional unit suffix beside the value.
func TestRenderStatUnit(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stat{Label: "Download", Value: "300", Unit: "Mbps", Icon: "speed"})
	if !strings.Contains(got, "Mbps") {
		t.Errorf("unit not rendered: %s", got)
	}
}

// TestRenderStatVariantTintsIconNotValue confirms the semantic variant colours the
// icon (state cue) while the value stays ink (consistent figures) — the ADR-005 rule.
func TestRenderStatVariantTintsIconNotValue(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stat{Label: "Protection", Value: "12", Icon: "shield", Variant: "good"})
	if !strings.Contains(got, "text-green-600") {
		t.Errorf("good variant did not tint the icon: %s", got)
	}
	if !strings.Contains(got, "text-slate-900") {
		t.Errorf("value should stay ink regardless of variant: %s", got)
	}
}

// TestRenderStatEscapes confirms shell-side escaping of plugin-supplied text.
func TestRenderStatEscapes(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stat{Label: "x", Value: `<script>alert(1)</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("stat value not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("escaped value missing: %s", got)
	}
}

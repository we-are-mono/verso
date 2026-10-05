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
		Label: "Devices", Value: "9", Sub: "online now", Icon: "devices", Variant: "success", Dot: true,
	})
	for _, want := range []string{
		"Devices", ">9", "online now",
		"rounded-xs border border-rule", // the one flat hairline frame, 2px corners
		"tabular-nums",                  // figures align
		"verso-live-dot",                // the pulsing dot when success
		"bg-green",                      // dot tinted by the success variant
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stat missing %q:\n%s", want, got)
		}
	}
	linked := render(t, r, &Stat{Label: "Devices", Value: "9", Href: "/devices"})
	for _, tile := range []string{got, linked} {
		for _, never := range []string{"rounded-xl", "shadow", "-translate-y-px"} {
			if strings.Contains(tile, never) {
				t.Errorf("a tile is flat — no %q:\n%s", never, tile)
			}
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
	got := render(t, r, &Stat{Label: "Protection", Value: "12", Icon: "shield", Variant: "success"})
	if !strings.Contains(got, "text-green") {
		t.Errorf("success variant did not tint the icon: %s", got)
	}
	if !strings.Contains(got, "text-ink") {
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

// TestABareStatIsAFigure: a bare stat is a vital laid on the open canvas (a
// box's processor temperature, its power draw), so its number is the canvas's
// figure — Inconsolata at 36px, the home page's meter's — with its unit quiet
// beside it, rather than a 16px number a reader has to look for.
func TestABareStatIsAFigure(t *testing.T) {
	got := render(t, newRenderer(t), &Stat{Style: "bare", Label: "Processor", Icon: "thermometer", Value: "76", Unit: "°C"})
	for _, want := range []string{
		`<div class="mt-3 flex items-baseline gap-1.5 leading-none">`,
		`<span class="font-mono text-4xl font-bold tracking-[-.05em] tabular-nums text-ink" data-verso-stat-v>76</span>`,
		`<span class="text-base text-meta" data-verso-stat-u>°C</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a bare stat is not a figure; missing %s:\n%s", want, got)
		}
	}
}

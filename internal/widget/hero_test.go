// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRenderHero(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Hero{Variant: "success", Title: "Everything's good", Body: "All quiet."})
	for _, want := range []string{
		"Everything&#39;s good", "All quiet.",
		"rounded-2xl",         // largest surface, largest radius
		"bg-emerald-600",      // success palette on the glyph
		"font-serif",          // verdict set in the display face
		`stroke-width="1.75"`, // Lucide glyph
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hero missing %q:\n%s", want, got)
		}
	}
}

// TestHeroVariantIconDefault: a variant with no explicit icon uses its default glyph.
func TestHeroVariantIconDefault(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Hero{Variant: "danger", Title: "Down"})
	if !strings.Contains(got, "bg-red-600") {
		t.Errorf("danger palette missing:\n%s", got)
	}
	if !strings.Contains(got, "m21.73 18") { // triangle-alert path
		t.Errorf("danger default glyph (triangle-alert) missing:\n%s", got)
	}
}

// TestHeroIconOverride: an explicit icon wins over the variant default (a calm check
// on an amber "one thing to check").
func TestHeroIconOverride(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Hero{Variant: "warning", Icon: "check", Title: "Calm"})
	if !strings.Contains(got, "M20 6 9 17l-5-5") { // check path
		t.Errorf("icon override (check) not applied:\n%s", got)
	}
}

// TestDecodeHeroChildren covers UnmarshalJSON + the CTA slot composing a link.
func TestDecodeHeroChildren(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"hero","variant":"danger","title":"Internet down",
		"children":[{"type":"link","label":"Restart","href":"/net","style":"button"}]}`))
	if err != nil {
		t.Fatalf("decode hero: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"Internet down", "Restart", `href="/net"`} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded hero missing %q:\n%s", want, got)
		}
	}
}

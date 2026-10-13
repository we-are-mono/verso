// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestASectionsGlyphLeadsItsTitle: a page's section is a card, and the card's
// header leads with the glyph of what the section is, in Glyph, as the
// Overview's cards do (ADR-023); a section without one shows none.
func TestASectionsGlyphLeadsItsTitle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "SSH", Icon: "terminal"})
	band := strings.Index(got, "data-verso-section-band")
	glyph := strings.Index(got, `<span aria-hidden="true" class="flex shrink-0 text-glyph"><svg`)
	title := strings.Index(got, ">SSH</h2>")
	if band < 0 || !(band < glyph && glyph < title) {
		t.Errorf("the glyph does not lead the title in the band:\n%s", got)
	}
	if bare := render(t, r, &Section{Title: "SSH"}); strings.Contains(bare, "text-glyph") {
		t.Errorf("a section without a glyph shows one:\n%s", bare)
	}
}

// TestAnArtifactNamesItsFrame: an artifact (a certificate) draws its own
// frame, which a page card takes away so no card holds a card (ADR-023); the
// frame says what it is so the stylesheet can find it.
func TestAnArtifactNamesItsFrame(t *testing.T) {
	got := render(t, newRenderer(t), &Card{Style: "artifact", Title: "HTTPS certificate"})
	if !strings.Contains(got, "<section data-verso-artifact class=") {
		t.Errorf("the artifact's frame is not named:\n%s", got)
	}
}

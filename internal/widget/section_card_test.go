// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestASectionsGlyphLeadsItsTitle: a page's section is a card, and the card's
// header leads with the glyph of what the section is, in Glyph, as the
// Overview's cards do; a section without one shows none.
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

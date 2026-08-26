// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderSectionSub: the description belongs to the heading — tight beneath
// the title (mb-1 instead of mb-3), with the saved space moved below the sub so
// the title-to-content distance is preserved. Markdown renders; without a sub
// the title keeps its original spacing.
func TestRenderSectionSub(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{
		Title:    "Zones",
		Sub:      "**Input** governs traffic to the router.",
		Children: []Widget{&Text{Markdown: "body"}},
	})
	for _, want := range []string{
		"mb-1",                   // title pulled tight to its description
		"mb-6",                   // the gap moves below the sub
		"<strong>Input</strong>", // sub renders Markdown
		"text-slate-500",         // sub is muted head-matter, not body prose
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section sub missing %q:\n%s", want, got)
		}
	}

	plain := render(t, r, &Section{Title: "Zones", Children: []Widget{&Text{Markdown: "body"}}})
	if !strings.Contains(plain, "mb-3") || strings.Contains(plain, "mb-6") {
		t.Errorf("section without sub should keep its original title spacing:\n%s", plain)
	}
}

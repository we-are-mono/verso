// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestASectionSaysWhatItIsNotHowItStands: a section states its kind and the
// stylesheet (sections.css) owns its air and its rule, so a page and a drawer
// draw the same section the same way, each by its own reach. No spacing or
// rule utility rides the element.
func TestASectionSaysWhatItIsNotHowItStands(t *testing.T) {
	r := newRenderer(t)
	child := []Widget{&Text{Markdown: "x"}}
	for kind, s := range map[string]*Section{
		"ruled":     {Title: "Time", Hairline: true, Children: child},
		"continued": {Hairline: true, Flush: true, Children: child},
		"plain":     {Title: "Time", Children: child},
		"bare":      {Title: "Time", Flush: true, Children: child},
	} {
		got := render(t, r, s)
		if !strings.Contains(got, `data-verso-section="`+kind+`"`) {
			t.Errorf("%s: section must say its kind:\n%s", kind, got)
		}
		open := got[:strings.Index(got, ">")]
		for _, utility := range []string{"mt-", "pt-", "border-t", "scroll-mt"} {
			if strings.Contains(open, utility) {
				t.Errorf("%s: the section carries no spacing of its own (%q):\n%s", kind, utility, open)
			}
		}
	}
	// A section inside a section is a part: ruled the same way, kept to the
	// content's inset.
	part := render(t, r, &Section{Title: "Outer", Children: []Widget{&Section{Title: "Inner", Hairline: true, Children: child}}})
	if !strings.Contains(part, `data-verso-section="part"`) {
		t.Errorf("a ruled section inside a section is a part:\n%s", part)
	}
}

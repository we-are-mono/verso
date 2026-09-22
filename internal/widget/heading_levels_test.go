// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestHeadingsFollowThePage: the page title is the page's one h1, so what a
// page is divided into is h2 (a section's title, a table's band, the kicker
// that heads a rail), and a dialog, which is a place of its own, is titled at
// h2 too. A skipped level reads to a screen reader as a missing section. The
// look is the classes', not the tag's, so retagging moves nothing on screen.
func TestHeadingsFollowThePage(t *testing.T) {
	r := newRenderer(t)
	cases := map[string]struct {
		w    Widget
		want string
	}{
		"section": {&Section{Title: "Privacy"}, `<h2 class="text-lg leading-tight font-semibold tracking-[-0.025em] text-ink">Privacy</h2>`},
		"kicker":  {&Section{Title: "On this page", Kicker: true}, `<h2 class="text-xs font-medium tracking-[.08em] text-meta uppercase">On this page</h2>`},
		"band":    {&Table{Title: "Leases", Columns: []TableColumn{{Label: "Name"}}}, `<h2 class="text-lg font-semibold tracking-tight text-body">Leases</h2>`},
		"drawer":  {&Drawer{Title: "Edit rule", Trigger: []Widget{&Text{Markdown: "Edit"}}}, `<h2 class="min-w-0 truncate text-lg font-semibold tracking-tight text-body">Edit rule</h2>`},
		"modal":   {&Modal{Title: "Restart", Trigger: "Restart"}, `<h2 class="text-base font-semibold text-ink">Restart</h2>`},
	}
	for name, c := range cases {
		got := render(t, r, c.w)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s in:\n%s", name, c.want, got)
		}
		if strings.Contains(got, "<h3") {
			t.Errorf("%s: no h3 at this level:\n%s", name, got)
		}
	}
}

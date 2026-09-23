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

// TestNestedSectionIsASubheading: a section inside a section is a part of that
// subject, not a subject of the page — an h3, set as a label rather than a
// second 18px heading beside its parent's. Depth counts sections only, so a
// section reached through a form is still nested.
func TestNestedSectionIsASubheading(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "SSH", Children: []Widget{
		&Form{Style: "settings", Submit: "Save", Fields: []Widget{
			&Section{Title: "Authorized keys", Children: []Widget{&Text{Markdown: "None."}}},
		}},
	}})
	for _, want := range []string{
		`<h2 class="text-lg leading-tight font-semibold tracking-[-0.025em] text-ink">SSH</h2>`,
		`<h3 class="text-sm font-semibold text-ink">Authorized keys</h3>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	sibling := render(t, r, &Stack{Children: []Widget{&Section{Title: "A"}, &Section{Title: "B"}}})
	if strings.Contains(sibling, "<h3") {
		t.Errorf("sections side by side stay h2:\n%s", sibling)
	}
}

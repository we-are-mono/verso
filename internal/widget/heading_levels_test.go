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

// TestSubheadingIsALedgerLine: a part of a section reads as the start of a
// group, never as one more field label: its name, then how many things the
// part holds, stand on a hairline that runs on to the column's end. The count
// is the set's own length, so it cannot disagree with what is listed under it,
// and it carries a key the page uses to notice the number change.
func TestSubheadingIsALedgerLine(t *testing.T) {
	keys := func(n int) *Collection {
		c := &Collection{Empty: "No keys yet."}
		for i := 0; i < n; i++ {
			c.Items = append(c.Items, CollectionItem{Title: "demo@laptop", Detail: "SHA256:x"})
		}
		return c
	}
	for n, want := range map[int]string{2: ">2</span>", 0: ">0</span>"} {
		got := render(t, newRenderer(t), &Section{Title: "SSH", Children: []Widget{
			&Section{Title: "Authorized keys", Children: []Widget{keys(n)}},
		}})
		for _, part := range []string{
			// 24px to the first box under it, as a form's rows stand apart
			`<div data-verso-ledger class="mb-6 flex items-center gap-3">`,
			`<h3 class="shrink-0 text-sm leading-5 font-semibold text-ink">Authorized keys</h3>`,
			`<span data-verso-count="Authorized keys" class="inline-grid h-5 shrink-0 overflow-hidden text-sm leading-5 font-medium tabular-nums text-meta"><span data-verso-count-value class="[grid-area:1/1]"`,
			want,
			`<span aria-hidden="true" class="h-px min-w-6 flex-1 bg-rule"></span>`,
		} {
			if !strings.Contains(got, part) {
				t.Errorf("%d keys: ledger line missing %s in:\n%s", n, part, got)
			}
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
		`<h3 class="shrink-0 text-sm leading-5 font-semibold text-ink">Authorized keys</h3>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// the part stands off by its parent's block rhythm alone, not by a
	// section's own lead-in on top of it
	if n := strings.Count(got, "pt-6"); n != 1 {
		t.Errorf("only the outer section takes the lead-in, got %d:\n%s", n, got)
	}
	// a part without a set to count still stands on its ledger line
	if !strings.Contains(got, `<span aria-hidden="true" class="h-px min-w-6 flex-1 bg-rule"></span>`) || strings.Contains(got, "data-verso-count") {
		t.Errorf("the part stands on its ledger line, with no count of nothing:\n%s", got)
	}
	sibling := render(t, r, &Stack{Children: []Widget{&Section{Title: "A"}, &Section{Title: "B"}}})
	if strings.Contains(sibling, "<h3") {
		t.Errorf("sections side by side stay h2:\n%s", sibling)
	}
}

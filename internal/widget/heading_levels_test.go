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
		"section": {&Section{Title: "Privacy"}, `<h2 class="text-lg leading-6 font-semibold tracking-[-0.025em] text-ink">Privacy</h2>`},
		"kicker":  {&Section{Title: "On this page", Kicker: true}, `<h2 class="text-xs leading-6 font-medium tracking-[.08em] text-meta uppercase">On this page</h2>`},
		"band":    {&Table{Title: "Leases", Columns: []TableColumn{{Label: "Name"}}}, `<h2 class="verso-listing__title">Leases</h2>`},
		"drawer":  {&Table{Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{Cells: []TableCell{{Text: "Rule"}}, Drawer: &RowDrawer{Title: "Edit rule", Open: true}}}}, `<h2 class="verso-modal__title">Edit rule</h2>`},
		"modal":   {&Modal{Title: "Restart", Trigger: "Restart"}, `<h2 class="verso-modal__title">Restart</h2>`},
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
	// A table's band title is the listing block's: the title role in Body.
	declares(t, "listing", ".verso-listing__title", "@apply type-title", "color: var(--color-body)")
	// A dialog's title is the title role in Ink, as a drawer's is.
	declares(t, "modal", ".verso-modal__title", "@apply type-title", "color: var(--color-ink)")
	// A drawer's title keeps to its one line, however long the name.
	declares(t, "modal", `.verso-modal__title:where(.verso-modal[data-verso-kind="drawer"] *)`,
		"min-width: 0", "overflow: hidden", "text-overflow: ellipsis", "white-space: nowrap")
}

// TestSubheadingIsALedgerLine: a part of a section reads as the start of a
// group, never as one more field label: its name stands on a hairline that
// runs on to the column's end. The part is never counted.
func TestSubheadingIsALedgerLine(t *testing.T) {
	keys := &Collection{Empty: "No keys yet.", Items: []CollectionItem{
		{Title: "demo@laptop", Detail: "SHA256:x"}, {Title: "demo@phone", Detail: "SHA256:y"},
	}}
	got := render(t, newRenderer(t), &Section{Title: "SSH", Children: []Widget{
		&Section{Title: "Authorized keys", Children: []Widget{keys}},
	}})
	for _, part := range []string{
		// one 20px line, and a cell to the first box under it, as a form's
		// rows stand apart
		`<div data-verso-ledger class="mb-6 flex h-6 items-center gap-3">`,
		`<h3 class="shrink-0 text-sm leading-6 font-semibold text-ink">Authorized keys</h3>`,
		// the strong hairline, so the ledger is told from the rows under it
		`<span aria-hidden="true" class="h-px min-w-6 flex-1 bg-rule-strong"></span>`,
	} {
		if !strings.Contains(got, part) {
			t.Errorf("ledger line missing %s in:\n%s", part, got)
		}
	}
	if strings.Contains(got, "data-verso-count") || strings.Contains(got, ">2</span>") {
		t.Errorf("a part counts what it holds:\n%s", got)
	}
	// A part's lede belongs to its ledger line, as a card's lede belongs to
	// its title: it stands straight under the line, and the cell to the
	// first box under the part stands under the lede.
	got = render(t, newRenderer(t), &Section{Title: "Where it may go", Children: []Widget{
		&Section{Title: "Named elsewhere", Sub: "A new name carries to each of them.", Children: []Widget{keys}},
	}})
	if !strings.Contains(got, `<div data-verso-ledger class="flex h-6 items-center gap-3">`) || !strings.Contains(got, `data-verso-section-lede class="verso-prose text-body mb-6"`) {
		t.Errorf("a part's lede does not stand straight under its ledger line:\n%s", got)
	}
	// A part with a glyph leads with it, in Glyph, before its name.
	got = render(t, newRenderer(t), &Section{Title: "Row", Children: []Widget{
		&Section{Title: "DHCP server", Icon: "server", Meta: "lan", Children: []Widget{&Text{Markdown: "x"}}},
	}})
	if !strings.Contains(got, `<div data-verso-ledger class="mb-6 flex h-6 items-center gap-3"><span class="shrink-0 text-glyph"><svg class="size-4"`) {
		t.Errorf("a part's glyph leads its ledger line:\n%s", got)
	}
	if !strings.Contains(got, `</svg></span><h3 class="shrink-0 text-sm leading-6 font-semibold text-ink">DHCP server</h3>`) {
		t.Errorf("the name follows the glyph:\n%s", got)
	}
	// The glyph survives the schema: a plugin's section decodes with it.
	decoded, err := Decode([]byte(`{"type":"section","title":"Device","icon":"git-merge","meta":"br-lan","children":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if section, ok := decoded.(*Section); !ok || section.Icon != "git-merge" {
		t.Errorf("a decoded section keeps its icon: %+v", decoded)
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
		`<h2 class="text-lg leading-6 font-semibold tracking-[-0.025em] text-ink">SSH</h2>`,
		`<h3 class="shrink-0 text-sm leading-6 font-semibold text-ink">Authorized keys</h3>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// the part stands off by its parent's block rhythm alone, not by a
	// section's own lead-in on top of it
	if n := strings.Count(got, `data-verso-section="bare"`); n != 1 || strings.Count(got, `data-verso-section="plain"`) != 1 {
		t.Errorf("only the outer section takes the lead-in; the part is bare:\n%s", got)
	}
	// a part without a set to count still stands on its ledger line
	if !strings.Contains(got, `<span aria-hidden="true" class="h-px min-w-6 flex-1 bg-rule-strong"></span>`) || strings.Contains(got, "data-verso-count") {
		t.Errorf("the part stands on its ledger line, with no count of nothing:\n%s", got)
	}
	sibling := render(t, r, &Stack{Children: []Widget{&Section{Title: "A"}, &Section{Title: "B"}}})
	if strings.Contains(sibling, "<h3") {
		t.Errorf("sections side by side stay h2:\n%s", sibling)
	}
}

func TestDrawerSectionsStartTheirOwnHierarchy(t *testing.T) {
	for _, kind := range []string{"row", "actionbar"} {
		t.Run(kind, func(t *testing.T) {
			body := []Widget{&Form{Fields: []Widget{
				&Section{Title: "Security", Children: []Widget{&Section{Title: "Encryption"}}},
			}}}
			panel := &RowDrawer{Title: "Edit network", Open: true, Children: body}
			var opener Widget
			switch kind {
			case "row":
				opener = &Table{Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{Cells: []TableCell{{Text: "Network"}}, Drawer: panel}}}
			case "actionbar":
				opener = &ActionBar{Action: &TableAction{Label: "Add network", Href: "?open=new"}, Drawer: panel}
			}
			tree := &Section{Title: "Wireless", Children: []Widget{
				opener,
				&Section{Title: "Page subsection"},
			}}
			r := newRenderer(t)
			got := render(t, r, tree)
			for _, want := range []string{
				`<h2 class="text-lg leading-6 font-semibold tracking-[-0.025em] text-ink">Security</h2>`,
				`<h3 class="shrink-0 text-sm leading-6 font-semibold text-ink">Encryption</h3>`,
				`<h3 class="shrink-0 text-sm leading-6 font-semibold text-ink">Page subsection</h3>`,
			} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %s in:\n%s", want, got)
				}
			}
			var fragment strings.Builder
			found, err := r.RenderOpenPanelWithToken(&fragment, tree, "", "", nil, Flash{})
			if err != nil || !found {
				t.Fatalf("open panel: found=%v, err=%v", found, err)
			}
			if !strings.Contains(fragment.String(), `text-ink">Security</h2>`) || !strings.Contains(fragment.String(), `text-ink">Encryption</h3>`) {
				t.Errorf("panel fragment hierarchy differs from the full page:\n%s", fragment.String())
			}
		})
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestDrawerDefaults: the reading-width panel and the framed card trigger.
func TestDrawerDefaults(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Drawer{Title: "Detail", Trigger: []Widget{&Row{Title: "open me"}}})
	// The framed trigger wears the app's one radius, the same 2px every other
	// boxed surface takes.
	for _, want := range []string{"max-w-md", "rounded-xs border border-rule", "open me", "Detail", "verso-drawer-scrollbar"} {
		if !strings.Contains(got, want) {
			t.Errorf("drawer missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "max-w-3xl") {
		t.Error("default drawer must not be wide")
	}
}

// TestDrawerWideBare: the wide panel for detail views, the bare trigger for
// rows living in a hairline-divided list — no card frame, the row hover tint.
func TestDrawerWideBare(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Drawer{Title: "Device", Size: "wide", Style: "bare",
		Trigger: []Widget{&Row{Title: "family-laptop", Chevron: true}}})
	for _, want := range []string{"max-w-3xl", "hover:bg-quiet", chevronPath} {
		if !strings.Contains(got, want) {
			t.Errorf("wide bare drawer missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "border border-rule bg-ground px-4") {
		t.Error("bare trigger must not wear the card frame")
	}
}

func TestDrawerHeadingAndTabsStayLocalized(t *testing.T) {
	tr := fakeCatalog(map[string]string{
		"New rule": "Novo pravilo", "Match": "Ujemanje", "Any": "Karkoli",
		"Close": "Zapri", "Sections": "Razdelki", "Save": "Shrani",
		"Name": "Ime", "name-from-config": "must not translate an identity",
	})
	r := newRenderer(t)
	if err := r.SetLanguages([]string{"sl"}, func(string) func(string) string { return tr }); err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		d := &RowDrawer{
			Title: "New rule", Open: true, HideTitle: true,
			Sub: "obsolete subtitle", Lede: []string{"obsolete recap"},
			Tabs:     []DrawerTab{{Label: "Match", State: "Any", Href: "/panel?tab=match", Active: true}},
			Children: []Widget{&Form{Fields: []Widget{&Field{Name: "name", Label: "Name", Value: "name-from-config"}}}},
		}
		var tree Widget = &ActionBar{Drawer: d}
		wantTitle := "Novo pravilo"
		if existing {
			d.Title, d.Verbatim = "name-from-config", true
			tree = &Table{Rows: []TableRow{{Drawer: d}}}
			wantTitle = "name-from-config"
		}
		var out strings.Builder
		found, err := r.RenderOpenPanelWithToken(&out, tree, "csrf-token", "sl", tr, Flash{})
		if err != nil || !found {
			t.Fatalf("existing=%v: found=%v, err=%v", existing, found, err)
		}
		html := out.String()
		for _, want := range []string{wantTitle + "</h2>", `aria-label="Zapri"`, `aria-label="Razdelki"`, "Ujemanje", "Karkoli", `value="name-from-config"`, `value="csrf-token"`, "Shrani", `hx-get="/panel?tab=match"`} {
			if !strings.Contains(html, want) {
				t.Errorf("existing=%v: localized drawer missing %q", existing, want)
			}
		}
		for _, obsolete := range []string{"obsolete subtitle", "obsolete recap", `class="sr-only"`, "must not translate an identity"} {
			if strings.Contains(html, obsolete) {
				t.Errorf("existing=%v: drawer contains %q", existing, obsolete)
			}
		}
	}
}

// chevronPath is the chevron-right icon's path data — the icon renders as
// inline SVG, so its name never appears in output.
const chevronPath = "m9 18 6-6-6-6"

// TestRowChevronIsOptIn: a plain informational row carries no "this opens"
// cue; a trigger row asks for it explicitly.
func TestRowChevronIsOptIn(t *testing.T) {
	r := newRenderer(t)
	if got := render(t, r, &Row{Title: "plain"}); strings.Contains(got, chevronPath) {
		t.Errorf("plain row must not render a chevron:\n%s", got)
	}
	if got := render(t, r, &Row{Title: "opens", Chevron: true}); !strings.Contains(got, chevronPath) {
		t.Errorf("trigger row missing its chevron:\n%s", got)
	}
}

// TestRenderOpenPanelWithToken: a request that is not asking for a page gets the
// open panel's contents and nothing around them — the frame is already on screen
// and only what it holds is being replaced. A tree with no open panel reports so
// rather than inventing one, and the caller then renders the page.
func TestRenderOpenPanelWithToken(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{
			{ID: "r1", Cells: []TableCell{{Text: "Allow-Ping"}}, Panel: "/x?open=r1"},
			{ID: "r2", Cells: []TableCell{{Text: "Allow-DHCP"}}, Drawer: &RowDrawer{
				Title: "Allow-DHCP", Open: true, Size: "wide",
				Tabs:     []DrawerTab{{Label: "Match", State: "5 conditions", Active: true}},
				Children: []Widget{&Callout{Body: "the reading"}},
			}},
		},
	}
	var b strings.Builder
	found, err := r.RenderOpenPanelWithToken(&b, tbl, "tok", "", nil, Flash{})
	if err != nil {
		t.Fatalf("RenderOpenPanelWithToken: %v", err)
	}
	if !found {
		t.Fatal("the open panel was not found")
	}
	got := b.String()
	for _, want := range []string{"Allow-DHCP", "the reading", `aria-label="Sections"`, "5 conditions"} {
		if !strings.Contains(got, want) {
			t.Errorf("panel contents missing %q:\n%s", want, got)
		}
	}
	// The frame, the scrim and the listing around them are already on screen.
	for _, unwanted := range []string{"x-teleport", `role="dialog"`, "Allow-Ping", "<table", "verso-flash"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a panel's contents must not carry %q:\n%s", unwanted, got)
		}
	}

	// A listing whose rows only carry frames has no open panel: the request is
	// for a page after all, and the caller renders one.
	frames := &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows:    []TableRow{{ID: "r1", Cells: []TableCell{{Text: "Allow-Ping"}}, Panel: "/x?open=r1"}},
	}
	var none strings.Builder
	switch found, err := r.RenderOpenPanelWithToken(&none, frames, "tok", "", nil, Flash{}); {
	case err != nil:
		t.Fatalf("RenderOpenPanelWithToken: %v", err)
	case found:
		t.Errorf("a listing with no open panel must report none:\n%s", none.String())
	}
}

// TestOpenPanelLeadsWithItsOutcome: a panel answering its own submission says how
// it went at the top of its body — the flash the page would have shown, drawn in
// the panel because the page is not being drawn around it. It is the page's own
// flash, toned the same way, so a save reads the same wherever it lands.
func TestOpenPanelLeadsWithItsOutcome(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{{ID: "r1", Cells: []TableCell{{Text: "Allow-Ping"}}, Drawer: &RowDrawer{
			Title: "Allow-Ping", Open: true,
			Children: []Widget{&Form{Submit: "Save & apply", Fields: []Widget{&Field{Name: "src", Label: "From"}}}},
		}}},
	}
	var b strings.Builder
	if _, err := r.RenderOpenPanelWithToken(&b, tbl, "tok", "", nil, Flash{Variant: "success", Message: "Rule saved."}); err != nil {
		t.Fatalf("RenderOpenPanelWithToken: %v", err)
	}
	got := b.String()
	flash := strings.Index(got, `<div class="verso-flash`)
	form := strings.Index(got, "<form")
	if flash < 0 || !strings.Contains(got, "Rule saved.") || !strings.Contains(got, "border-green-line") {
		t.Fatalf("the panel should lead with its outcome in the flash treatment:\n%s", got)
	}
	if form < 0 || flash > form {
		t.Errorf("the outcome comes before the form it answers:\n%s", got)
	}
}

// TestPanelFormsPostIntoThePanel: every form an open panel carries posts back
// into that panel — a row's, and the bar's blank one alike — so a save swaps in
// where the form was instead of navigating the listing behind it. The listing's
// own forms stay ordinary.
func TestPanelFormsPostIntoThePanel(t *testing.T) {
	r := newRenderer(t)
	editing := func() Widget {
		return &Form{Submit: "Save & apply", Fields: []Widget{&Field{Name: "src", Label: "From"}}}
	}
	deleting := func() Widget {
		return &Form{NoSubmit: true, Fields: []Widget{
			&Field{Name: "_delete", Kind: "hidden", Value: "1"},
			&Confirm{Trigger: "Delete rule", Message: "Delete it?", Confirm: "Delete rule"},
		}}
	}
	into := `hx-post="" hx-target="closest [data-verso-panel]"`

	row := render(t, r, &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{{ID: "r1", Cells: []TableCell{{Text: "Allow-Ping"}}, Drawer: &RowDrawer{
			Title: "Allow-Ping", Open: true, Children: []Widget{editing(), deleting()},
		}}},
	})
	if n := strings.Count(row, into); n != 2 {
		t.Errorf("both of a row panel's forms should post into the panel, %d do:\n%s", n, row)
	}
	// And the frame answers a submission that is done by finishing.
	if !strings.Contains(row, `@verso-panel-done="finish"`) {
		t.Errorf("the frame should finish on a submission that is done:\n%s", row)
	}

	bar := render(t, r, &ActionBar{
		Action: &TableAction{Label: "Add rule", Href: "/x?open=new"}, OpensPanel: true,
		Drawer: &RowDrawer{Title: "New rule", Open: true, Children: []Widget{editing()}},
	})
	if !strings.Contains(bar, into) {
		t.Errorf("the bar's blank panel form should post into the panel:\n%s", bar)
	}

	page := render(t, r, &Stack{Children: []Widget{editing()}})
	if strings.Contains(page, "hx-post") {
		t.Errorf("a form outside any panel posts its page:\n%s", page)
	}
}

// TestRowShipsAnEmptyFrame: a row that names where its panel comes from renders
// the frame and not the panel — that is the whole point, so a listing of forty
// rows carries forty frames rather than forty panels nobody has asked for.
func TestRowShipsAnEmptyFrame(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{{ID: "r1", Panel: "/x?open=r1", Cells: []TableCell{
			{Text: "Allow-Ping", Href: "/x?open=r1"},
		}}},
	})
	for _, want := range []string{
		`data-verso-panel data-verso-panel-url="/x?open=r1"`,
		// The name is still a link, so the panel is reachable with no script at
		// all; the click handler is what keeps it from leaving the page.
		`<a href="/x?open=r1" @click.prevent="showPanel"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fetched-panel row missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "aria-label=\"Sections\"") {
		t.Errorf("the frame ships empty — no contents until it is opened:\n%s", got)
	}
}

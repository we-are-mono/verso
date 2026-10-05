// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestDrawerOneWidth: every drawer stands at the same 768px, whatever it
// holds — a form, a chooser or a reading — so the frame never changes size
// from one object to the next.
func TestDrawerOneWidth(t *testing.T) {
	r := newRenderer(t)
	for _, size := range []string{"", "choices"} {
		tbl := &Table{Columns: []TableColumn{{Label: "Name", Kind: "name"}}, Rows: []TableRow{{
			ID: "r1", Cells: []TableCell{{Text: "lan"}},
			Drawer: &RowDrawer{Title: "lan", Size: size, Children: []Widget{&Callout{Body: "x"}}},
		}}}
		got := render(t, r, tbl)
		if !strings.Contains(got, "max-w-3xl") {
			t.Errorf("drawer size %q is not the one width:\n%s", size, got)
		}
		for _, other := range []string{"max-w-md", "max-w-[40rem]", "data-verso-panel-size"} {
			if strings.Contains(got, other) {
				t.Errorf("drawer size %q carries a second width %q", size, other)
			}
		}
	}
}

func TestDrawerHeadingAndTabsStayLocalized(t *testing.T) {
	tr := fakeCatalog(map[string]string{
		"New rule": "Novo pravilo", "Match": "Ujemanje",
		"Close": "Zapri", "Sections": "Razdelki", "Save changes": "Shrani spremembe",
		"Name": "Ime", "name-from-config": "must not translate an identity",
	})
	r := newRenderer(t)
	if err := r.SetLanguages([]string{"sl"}, func(string) func(string) string { return tr }); err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		d := &RowDrawer{
			Title: "New rule", Open: true,
			Tabs:     []DrawerTab{{Label: "Match", Href: "/panel?tab=match", Active: true}},
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
		for _, want := range []string{wantTitle + "</h2>", `aria-label="Zapri"`, `aria-label="Razdelki"`, "Ujemanje", `value="name-from-config"`, `value="csrf-token"`, "Shrani spremembe", `hx-get="/panel?tab=match"`} {
			if !strings.Contains(html, want) {
				t.Errorf("existing=%v: localized drawer missing %q", existing, want)
			}
		}
		for _, obsolete := range []string{`class="sr-only"`, "must not translate an identity"} {
			if strings.Contains(html, obsolete) {
				t.Errorf("existing=%v: drawer contains %q", existing, obsolete)
			}
		}
	}
}

// TestRenderOpenPanelWithToken: a request that is not asking for a page gets the
// open panel's contents and nothing around them — the frame is already on screen
// and only what it holds is being replaced. A tree with no open panel reports so
// rather than inventing one, and the caller then renders the page.
// TestDrawerTabsAreTheirNames: a tab names a reading, as a field's label names
// a setting, so it is set at the drawer's 14px label scale, and it is its name
// alone — no chip of where the object stands beside it; the 18px section
// heading under the strip is the next step up, never a near neighbour.
func TestDrawerTabsAreTheirNames(t *testing.T) {
	tbl := &Table{Rows: []TableRow{{ID: "r", Cells: []TableCell{{Text: "lan"}}, Drawer: &RowDrawer{
		Title: "lan", Open: true,
		Tabs:     []DrawerTab{{Label: "Traffic", Active: true}, {Label: "Reaches"}},
		Children: []Widget{&Callout{Body: "the reading"}},
	}}}}
	var b strings.Builder
	if _, err := newRenderer(t).RenderOpenPanelWithToken(&b, tbl, "tok", "", nil, Flash{}); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if n := strings.Count(got, "items-center whitespace-nowrap text-sm leading-5 transition"); n != 2 {
		t.Errorf("want both tabs at the 14px label scale, found %d:\n%s", n, got)
	}
	if strings.Contains(got, "whitespace-nowrap text-base") {
		t.Error("a tab is set a step above the drawer's labels")
	}
	nav := got[strings.Index(got, "<nav"):strings.Index(got, "</nav>")]
	if strings.Contains(nav, "<span") {
		t.Errorf("a tab wears something beside its name: %s", nav)
	}
}

func TestRenderOpenPanelWithToken(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{
			{ID: "r1", Cells: []TableCell{{Text: "Allow-Ping"}}, Panel: "/x?open=r1"},
			{ID: "r2", Cells: []TableCell{{Text: "Allow-DHCP"}}, Drawer: &RowDrawer{
				Title: "Allow-DHCP", Open: true,
				Tabs:     []DrawerTab{{Label: "Match", Active: true}},
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
	for _, want := range []string{"Allow-DHCP", "the reading", `aria-label="Sections"`, ">Match</a>"} {
		if !strings.Contains(got, want) {
			t.Errorf("panel contents missing %q:\n%s", want, got)
		}
	}
	// The frame, the scrim and the listing around them are already on screen.
	for _, unwanted := range []string{"x-teleport", "<dialog", "Allow-Ping", "<table", "verso-flash"} {
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
		// The name is words, and a row with no acts keeps its trailing Details,
		// a link, so the panel is reachable with no script at all; the click
		// handler is what keeps it from leaving the page.
		`<a href="/x?open=r1" @click.prevent="showPanel"`,
		`>Details</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fetched-panel row missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "aria-label=\"Sections\"") {
		t.Errorf("the frame ships empty — no contents until it is opened:\n%s", got)
	}
}

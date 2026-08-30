// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// redirectsTable is the styleguide's Redirects listing in miniature: one
// zone-sourced DNAT to the router and one internet-facing port forward to a
// device — every column kind exercised once.
func redirectsTable() *Table {
	return &Table{
		Columns: []TableColumn{
			{Kind: "toggle"},
			{Label: "From", Kind: "endpoint"},
			{Label: "To", Kind: "endpoint"},
			{Label: "Protocol", Kind: "keyword"},
			{Label: "Port", Kind: "mono"},
			{Label: "Comment", Kind: "comment"},
			{Label: "Hits", Kind: "num"},
		},
		Rows: []TableRow{
			{ID: "force_dns_guest", Cells: []TableCell{
				{On: true, Name: "force_dns_guest"},
				{Endpoints: []TableEndpoint{{Kind: "zone", Label: "guest"}}},
				{Endpoints: []TableEndpoint{{Kind: "router", Label: "router"}}},
				{Text: "tcp/udp"},
				{Text: "53"},
				{Text: "Force-DNS-to-AdGuard-guest"},
				{Text: "0"},
			}},
			{ID: "https_to_nas", Cells: []TableCell{
				{On: false},
				{Endpoints: []TableEndpoint{{Kind: "zone", Label: "wan"}}},
				{Endpoints: []TableEndpoint{{Kind: "device", Label: "10.0.0.30"}}},
				{Text: "tcp"},
				{Text: "8443 → 443"},
				{Text: "HTTPS-to-NAS"},
				{Text: "1.2k"},
			}},
		},
	}
}

// TestRenderTableKinds: each column kind gets its one treatment — mono for
// machine strings, sans keywords, muted comments, right-aligned tabular
// counters, a switch for toggles.
func TestRenderTableKinds(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, redirectsTable())
	for _, want := range []string{
		">Protocol<", ">Hits<", // headers render
		"font-semibold",              // header treatment is weight, not a fill
		"font-mono",                  // the port column is a machine string
		"8443 → 443",                 // port rewrite verbatim
		"tabular-nums",               // counters align
		`type="checkbox"`,            // the toggle is a real switch
		`name="force_dns_guest"`,     // …posting under the section's handle
		"Force-DNS-to-AdGuard-guest", // comments carry the optional UCI name
		"text-right",                 // num columns right-align (th and td)
	} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q:\n%s", want, got)
		}
	}
	// " checked" is the bare attribute; the peer-checked: utility classes never
	// carry a leading space before "checked".
	if strings.Count(got, " checked") != 1 {
		t.Errorf("exactly one toggle should be checked:\n%s", got)
	}
}

func TestRenderEmphasisedMonoCell(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Label: "Address", Kind: "mono"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "10.0.0.232", Emphasis: true}}}},
	})
	for _, want := range []string{"font-mono", "text-lg", "font-semibold", "10.0.0.232"} {
		if !strings.Contains(got, want) {
			t.Errorf("emphasised mono cell missing %q:\n%s", want, got)
		}
	}
}

// TestRenderTableEndpoints: endpoints render as a type icon + label with one
// treatment per kind — zone sans, device mono, router accented. No pills: the
// grid contains the cells, so a badge would be redundant chrome.
func TestRenderTableEndpoints(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, redirectsTable())
	for _, want := range []string{
		"guest", "10.0.0.30", "router",
		"text-sky-700", // router (this device) carries the accent
	} {
		if !strings.Contains(got, want) {
			t.Errorf("endpoints missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "rounded-full border") {
		t.Errorf("endpoints must not render as pill chips inside a table:\n%s", got)
	}
	// The zone glyph (plain shield) and the device glyph (monitor) both appear.
	if !strings.Contains(got, lucideIcons["zone"]) {
		t.Errorf("zone endpoint should use the plain shield glyph:\n%s", got)
	}
	if !strings.Contains(got, lucideIcons["device"]) {
		t.Errorf("device endpoint should use the device glyph:\n%s", got)
	}
}

// TestRenderTableNameAndPill: a "name" column is the row's bold identity; a
// "pill" column renders enum values through the badge component (same variant
// vocabulary, same palette) and an empty pill cell reads as a faint dash.
func TestRenderTableNameAndPill(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{
			{Label: "Zone", Kind: "name"},
			{Label: "Input", Kind: "pill"},
			{Label: "NAT", Kind: "pill"},
		},
		Rows: []TableRow{
			{ID: "lan", Cells: []TableCell{{Text: "lan"}, {Text: "accept", Variant: "success"}, {}}},
			{ID: "wan", Cells: []TableCell{{Text: "wan"}, {Text: "reject", Variant: "warning"}, {Text: "NAT", Variant: "info"}}},
		},
	})
	for _, want := range []string{
		"font-semibold text-slate-900",          // the identity column is emphasised ink, no icon
		"bg-green-50 text-green-700",            // accept pill through the badge palette
		"bg-amber-50 text-amber-700",            // reject pill
		"bg-sky-50 text-sky-700",                // NAT carries the info accent
		`<span class="text-slate-300">—</span>`, // empty pill cell is a faint dash
	} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTableReferenceWithZoneChip(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Kind: "name"}, {Kind: "reference"}},
		Rows: []TableRow{{Cells: []TableCell{
			{Text: "Laptop"},
			{Text: "br-lan.10", Chips: []TableChip{{Icon: "zone", Label: "family"}}},
		}}},
	})
	if !strings.Contains(got, `<span class="text-slate-900">br-lan.10</span>`) {
		t.Errorf("referenced interface should use regular-weight identity text:\n%s", got)
	}
	if !strings.Contains(got, ">family</span>") || !strings.Contains(got, lucideIcons["zone"]) {
		t.Errorf("referenced interface should carry the shared zone chip:\n%s", got)
	}
	if !strings.Contains(got, "dark:bg-slate-400/10 dark:text-slate-700 dark:ring-slate-400/20") {
		t.Errorf("reference chips should carry the shared dark treatment:\n%s", got)
	}
}

// TestRenderTableSeam: the seam folds extra rows behind a native <details>
// inside the same card — a full-bleed divider, never a nested card — repeating
// the column head so the expanded block reads as more of the same listing.
func TestRenderTableSeam(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Style = "card"
	tbl.Seam = &TableSeam{
		Summary: "OpenWrt defaults — 9 stock rules",
		Rows: []TableRow{{ID: "allow_ping", Cells: []TableCell{
			{On: true}, {Endpoints: []TableEndpoint{{Kind: "zone", Label: "wan"}}},
			{Endpoints: []TableEndpoint{{Kind: "router", Label: "router"}}},
			{Text: "icmp"}, {Text: "echo-request"}, {Text: "Allow-Ping"}, {Text: "1.4k"},
		}}},
	}
	got := render(t, r, tbl)
	for _, want := range []string{
		"<details", "OpenWrt defaults — 9 stock rules", "Allow-Ping",
		"verso-chevron",     // the shared rotate-on-open affordance
		lucideIcons["lock"], // the stock-rules padlock
		"-mx-5 overflow-hidden rounded-b-2xl border-t border-slate-200", // full-bleed divider; hover wash clips to the card's bottom radius
	} {
		if !strings.Contains(got, want) {
			t.Errorf("seam missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "rounded-2xl") != 1 {
		t.Errorf("the seam must not add a second card:\n%s", got)
	}
	if strings.Count(got, "<thead>") != 2 {
		t.Errorf("the seam repeats the column head:\n%s", got)
	}
	if strings.Contains(render(t, r, redirectsTable()), "<details") {
		t.Error("a table without a seam should render no details element")
	}
}

// TestRenderTableRowDrawer: a row with a drawer is an openable object, but the
// row itself is inert — it hosts its own modal scope and opens from a trailing
// "Details" link (never a whole-row click, so cell values stay selectable). The
// panel teleports to <body>; forms inside it receive the CSRF token.
func TestRenderTableRowDrawer(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{
		Title:    "Edit redirect — Force-DNS-to-AdGuard-guest",
		Children: []Widget{&Form{Submit: "Save changes", Fields: []Widget{&Field{Name: "src", Label: "From zone", Value: "guest"}}}},
	}
	var b strings.Builder
	if err := r.RenderWithToken(&b, tbl, "tok123"); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := b.String()
	for _, want := range []string{
		`x-data="modal"`, `@click="show"`, ">Details<", // opens from the trailing link
		"x-teleport", "Edit redirect — Force-DNS-to-AdGuard-guest",
		"Save changes", `value="tok123"`, // the drawer's form carries the CSRF token
		"dark:bg-black/60",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("row drawer missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		`@click="showFromRow"`, // the row itself is not clickable
		"m9 18 6-6-6-6",        // no row chevron — the "Details" link is the affordance
	} {
		if strings.Contains(got, absent) {
			t.Errorf("inert row must not contain %q:\n%s", absent, got)
		}
	}
	if strings.Count(got, `x-data="modal"`) != 1 {
		t.Errorf("only drawer rows should host a modal scope:\n%s", got)
	}
}

// TestDecodeTableRowDrawer: the drawer's children decode recursively, and an
// unknown child fails loudly.
func TestDecodeTableRowDrawer(t *testing.T) {
	w, err := Decode([]byte(`{
		"type": "table",
		"columns": [{"label":"A"}],
		"rows": [{"id":"r1","cells":[{"text":"1"}],
			"drawer": {"title":"Edit","children":[{"type":"text","markdown":"body"}]}}]
	}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tb := w.(*Table)
	if tb.Rows[0].Drawer == nil || tb.Rows[0].Drawer.Title != "Edit" || len(tb.Rows[0].Drawer.Children) != 1 {
		t.Errorf("row drawer not decoded: %+v", tb.Rows[0].Drawer)
	}
	if _, err := Decode([]byte(`{"type":"table","columns":[],"rows":[{"cells":[],"drawer":{"title":"x","children":[{"type":"nope"}]}}]}`)); err == nil {
		t.Error("unknown drawer child should fail loudly")
	}
}

// TestTableRaggedRow: a row shorter than the column set still renders one cell
// per column, so the grid never collapses.
func TestTableRaggedRow(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Label: "A"}, {Label: "B", Kind: "num"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "only"}}}},
	})
	if strings.Count(got, "<td") != 2 {
		t.Errorf("short row should be padded to the column count:\n%s", got)
	}
}

// flatDevicesTable is the connected-devices flat listing in miniature: a header
// band, no column labels, and a drawer row whose only click target is the
// trailing "Details" link.
func flatDevicesTable() *Table {
	return &Table{
		Style:  "flat",
		Title:  "Connected devices",
		Detail: "12 online · 3 busy",
		Action: &TableAction{Label: "View all", Href: "/devices"},
		Columns: []TableColumn{
			{Kind: "name"}, {Kind: "keyword"}, {Kind: "pill"},
		},
		Rows: []TableRow{
			{Cells: []TableCell{{Text: "Gaming PC"}, {Text: "Ethernet"}, {Text: "High use", Variant: "warning"}},
				Drawer: &RowDrawer{Title: "Gaming PC", Children: []Widget{&Divider{}}}},
		},
	}
}

// TestRenderTableFlat: the flat style draws a header band aligned to the table's
// edges, no stripe, inert rows (values stay selectable), and opens a drawer from
// a trailing "Details" link rather than a whole-row click. A labelless flat table
// draws no <thead>.
func TestRenderTableFlat(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, flatDevicesTable())
	for _, want := range []string{
		">Connected devices<",       // header band title
		"12 online · 3 busy",        // header band detail
		">View all<",                // header band action
		`href="/devices"`,           // action link target
		">Details<",                 // the trailing "more" affordance
		`@click="show"`,             // the Details link opens the drawer
		"border-b border-slate-200", // hairline rows
	} {
		if !strings.Contains(got, want) {
			t.Errorf("flat table missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"odd:bg-slate-50",      // stripes are retired in the flat style
		`@click="showFromRow"`, // the row itself is not clickable
		"cursor-pointer",       // no pointer on inert rows
		"<thead",               // no column labels → no header row
	} {
		if strings.Contains(got, absent) {
			t.Errorf("flat table must not contain %q:\n%s", absent, got)
		}
	}
}

// TestRenderTableFlatLabels: a flat table WITH column labels renders the <thead>
// and stays stripeless.
func TestRenderTableFlatLabels(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Style:   "flat",
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Port", Kind: "mono"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "SSH"}, {Text: "22"}}}},
	})
	for _, want := range []string{"<thead", ">Name<", ">Port<"} {
		if !strings.Contains(got, want) {
			t.Errorf("labelled flat table missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "odd:bg-slate-50") {
		t.Errorf("flat table must not stripe:\n%s", got)
	}
}

// TestRenderTableFlatCondensed: Condensed lowers the flat row padding.
func TestRenderTableFlatCondensed(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Style: "flat", Condensed: true,
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "SSH"}}}},
	})
	if !strings.Contains(got, "[&_td]:py-1.5") {
		t.Errorf("condensed flat table should tighten row padding:\n%s", got)
	}
}

// TestRenderTableCellButton: a cell can carry a button that opens the row's
// drawer in place of the trailing "Details" link. When every drawer row opens
// from such a button, the table grows no trailing Details column at all.
func TestRenderTableCellButton(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Style:   "flat",
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Static", Kind: "pill"}},
		Rows: []TableRow{
			// reserved: a badge, no drawer.
			{Cells: []TableCell{{Text: "nas"}, {Text: "reserved", Variant: "info"}}},
			// dynamic: an in-cell "Reserve IP" button that opens the drawer.
			{Cells: []TableCell{{Text: "laptop"}, {Button: "Reserve IP"}},
				Drawer: &RowDrawer{Title: "Reserve — laptop", Children: []Widget{&Divider{}}}},
		},
	})
	for _, want := range []string{
		"reserved",                      // the badge state
		">Reserve IP<", `@click="show"`, // the action button opens the drawer
		`x-data="modal"`, // the button's row hosts the modal scope
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cell-button table missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, ">Details<") {
		t.Errorf("no trailing Details when the drawer opens from an in-cell button:\n%s", got)
	}
}

// TestRenderTableDirectAction: an immediate row command posts its action marker
// from the standard alert dialog without manufacturing an edit drawer.
func TestRenderTableDirectAction(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Style:   "flat",
		Columns: []TableColumn{{Label: "Session", Kind: "name"}, {Label: "Access", Kind: "pill"}},
		Rows: []TableRow{{ID: "iphone", Cells: []TableCell{
			{Text: "Safari · iPhone"},
			{Button: "End session", Action: "end-session:iphone", ConfirmTitle: "End this session?", Confirm: "Anyone using it will be signed out."},
		}}},
	})
	for _, want := range []string{
		`method="post"`, `name="_action"`, `value="end-session:iphone"`,
		`role="alertdialog"`, `>End this session?<`, "Anyone using it will be signed out.",
		"bg-red-100 text-red-600", `>End session<`, `>Cancel<`,
		"text-slate-600 transition-colors hover:bg-slate-100", // link-style Cancel
		"dark:border-gray-700 dark:bg-transparent dark:text-gray-300",
		"dark:bg-red-800 dark:text-gray-100 dark:hover:bg-red-900 dark:active:bg-red-950",
		"active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0",
		"dark:text-gray-300 dark:hover:bg-gray-800 dark:hover:text-gray-200 dark:active:bg-gray-900",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("direct table action missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<aside`) {
		t.Errorf("direct table action must not create a drawer:\n%s", got)
	}
	if strings.Contains(got, "sm:justify-end") {
		t.Errorf("alert actions must stay left aligned:\n%s", got)
	}
}

// TestDecodeTable: the wire shape — kind-typed columns, cells with endpoints —
// round-trips through Decode.
func TestDecodeTable(t *testing.T) {
	w, err := Decode([]byte(`{
		"type": "table",
		"columns": [{"kind":"toggle"},{"label":"From","kind":"endpoint"},{"label":"Hits","kind":"num"}],
		"rows": [{"id":"r1","cells":[
			{"on":true,"name":"r1"},
			{"endpoints":[{"kind":"zone","label":"guest"},{"kind":"device","label":"10.0.0.30"}]},
			{"text":"28"}
		]}]
	}`))
	if err != nil {
		t.Fatalf("decode table: %v", err)
	}
	tb, ok := w.(*Table)
	if !ok {
		t.Fatalf("decoded %T, want *Table", w)
	}
	if len(tb.Columns) != 3 || tb.Columns[1].Kind != "endpoint" {
		t.Errorf("columns not decoded: %+v", tb.Columns)
	}
	if tb.Rows[0].ID != "r1" || len(tb.Rows[0].Cells[1].Endpoints) != 2 {
		t.Errorf("rows not decoded: %+v", tb.Rows)
	}
}

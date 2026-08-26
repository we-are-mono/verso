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
		"uppercase",                  // header treatment is typographic, not a fill
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
		"font-bold",                             // the identity column is bold ink, no icon
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

// TestRenderTableRowDrawer: a row with a drawer is an openable object — it hosts
// its own modal scope, opens via showFromRow (which skips clicks on controls),
// shows the chevron affordance, and teleports its panel to <body>. Rows without
// drawers pad the chevron column so the grid stays aligned; forms inside the
// drawer receive the CSRF token.
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
		`x-data="modal"`, `@click="showFromRow"`, "cursor-pointer",
		"x-teleport", "Edit redirect — Force-DNS-to-AdGuard-guest",
		"Save changes", `value="tok123"`, // the drawer's form carries the CSRF token
	} {
		if !strings.Contains(got, want) {
			t.Errorf("row drawer missing %q:\n%s", want, got)
		}
	}
	// One chevron for the drawer row; the drawerless row pads the column.
	if strings.Count(got, "m9 18 6-6-6-6") != 1 {
		t.Errorf("exactly one chevron affordance expected:\n%s", got)
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

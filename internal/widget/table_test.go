// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"slices"
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
		"text-sky-700 dark:text-sky-400",    // router stays bright enough on dark surfaces
		"font-mono text-base font-semibold", // device addresses use the standard mono table type
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

func TestRenderPrimaryEndpointOneStepLarger(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{Cells: []TableCell{{Endpoints: []TableEndpoint{
			{Kind: "zone", Label: "guest"},
		}}}}},
	})
	if !strings.Contains(got, "whitespace-nowrap text-base font-semibold text-slate-900") {
		t.Errorf("primary endpoint must use the larger identity treatment:\n%s", got)
	}
}

func TestRenderReorderHandle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		ReorderConfig: "firewall",
		Columns:       []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{ID: "allow-dns", Cells: []TableCell{
			{}, {Endpoints: []TableEndpoint{{Kind: "zone", Label: "guest"}}},
		}}},
	})
	for _, want := range []string{
		"data-verso-reorder-table", `data-verso-reorder-config="firewall"`,
		"data-verso-reorder-row", "data-verso-reorder-handle",
		`aria-label="Reorder allow-dns"`, "cursor-grab", "active:cursor-grabbing",
		lucideIcons["grip-vertical"], "text-base font-semibold text-slate-900",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reorderable table missing %q:\n%s", want, got)
		}
	}
}

// TestRenderReorderOrderForm: a reorderable table renders the page form the
// dragged sequence lives in — the config, and one hidden input per row id in
// render order, wrapped as a change field so the staged-changes bar counts and
// reviews the pending order like any other edit.
func TestRenderReorderOrderForm(t *testing.T) {
	r := newRenderer(t)
	var b strings.Builder
	table := &Table{
		ReorderConfig: "firewall",
		ReorderLabel:  "Rule order",
		Columns:       []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{
			{ID: "allow_dhcp_renew", Cells: []TableCell{{}, {}}},
			{ID: "allow_ping", Cells: []TableCell{{}, {}}},
		},
		Seam: &TableSeam{Summary: "Stock rules", Rows: []TableRow{
			{ID: "block_telnet", Cells: []TableCell{{}, {}}},
		}},
	}
	if err := r.RenderWithToken(&b, table, "tok3n", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	got := normalizeHTML(b.String())
	for _, want := range []string{
		// data-verso-order-form marks a form that is not the page's whole
		// editable surface, so a staged discard reloads rather than swapping it.
		`<form method="post" data-verso-page-form data-verso-order-form hidden>`,
		`<input type="hidden" name="_csrf" value="tok3n">`,
		`<input type="hidden" name="` + ReorderConfigField + `" value="firewall">`,
		`data-verso-change-field data-verso-change-name="` + ReorderIDField + `"`,
		`data-verso-change-label="Rule order" data-verso-change-kind="order"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("order form missing %q:\n%s", want, got)
		}
	}
	// A listing that names no label falls back to the shell's generic word.
	var fallback strings.Builder
	unlabeled := &Table{
		ReorderConfig: "firewall",
		Columns:       []TableColumn{{Kind: "reorder"}},
		Rows:          []TableRow{{ID: "r1", Cells: []TableCell{{}}}},
	}
	if err := r.RenderWithToken(&fallback, unlabeled, "tok3n", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	if !strings.Contains(normalizeHTML(fallback.String()), `data-verso-change-label="Order"`) {
		t.Errorf("unlabeled order form should fall back to the generic label:\n%s", fallback.String())
	}
	// The sequence is the render order, seam rows included: the file holds one
	// order, so every draggable row is in it.
	var ids []string
	for _, part := range strings.Split(got, `name="`+ReorderIDField+`" value="`) {
		if id, _, ok := strings.Cut(part, `"`); ok && !strings.Contains(id, "<") {
			ids = append(ids, id)
		}
	}
	if want := []string{"allow_dhcp_renew", "allow_ping", "block_telnet"}; !slices.Equal(ids, want) {
		t.Errorf("order inputs = %v, want %v:\n%s", ids, want, got)
	}
}

// TestRenderTableWithoutReorderCarriesNoOrderForm: a listing that does not drag
// has no order to stage, so it composes no page form and cannot collide with one
// the page's own editor renders.
func TestRenderTableWithoutReorderCarriesNoOrderForm(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Label: "From", Kind: "endpoint"}},
		Rows:    []TableRow{{ID: "allow_ping", Cells: []TableCell{{}}}},
	})
	if strings.Contains(got, "data-verso-page-form") || strings.Contains(got, ReorderConfigField) {
		t.Errorf("a table that does not drag must render no order form:\n%s", got)
	}
}

// TestRenderReorderColumnWithoutConfigDrawsNoHandle: the drag persists through a
// uci order on the declared config, so a table that names none offers no grip —
// the column still holds its slot, keeping the grid identical to the live case.
func TestRenderReorderColumnWithoutConfigDrawsNoHandle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{ID: "allow-dns", Cells: []TableCell{
			{}, {Endpoints: []TableEndpoint{{Kind: "zone", Label: "guest"}}},
		}}},
	})
	for _, unwanted := range []string{
		"data-verso-reorder-table", "data-verso-reorder-config",
		"data-verso-reorder-row", "data-verso-reorder-handle",
		lucideIcons["grip-vertical"],
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("table with no reorder_config still carries %q:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, `class="w-8 border-b border-slate-200`) {
		t.Errorf("the reorder column should keep its slot:\n%s", got)
	}
}

// TestDecodeTableReorderConfig pins the wire name a plugin declares the drag
// under: the shell reads it to attach the interaction and to bound the uci
// config the reorder is staged against.
func TestDecodeTableReorderConfig(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","reorder_config":"firewall",` +
		`"columns":[{"kind":"reorder"},{"label":"From","kind":"endpoint"}],` +
		`"rows":[{"id":"allow-dns","cells":[{},{}]}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	table, ok := w.(*Table)
	if !ok {
		t.Fatalf("decoded %T, want *Table", w)
	}
	if table.ReorderConfig != "firewall" {
		t.Errorf("ReorderConfig = %q, want %q", table.ReorderConfig, "firewall")
	}
}

func TestRenderTableGroupHeader(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{ID: "allow-dns", Group: &TableGroup{
			Label: "Guest → Router", Chain: "input_guest", Count: 3,
		}, Cells: []TableCell{{}, {Endpoints: []TableEndpoint{{Kind: "zone", Label: "guest"}}}}}},
	})
	for _, want := range []string{
		`colspan="2"`, "Guest → Router", "input_guest", "· 3 rules",
		"verso-table-group bg-slate-50", "px-3 py-3",
		"font-mono text-base font-semibold text-slate-500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("grouped table missing %q:\n%s", want, got)
		}
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
		"bg-emerald-50 text-emerald-700",        // accept pill through the badge palette
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
	if !strings.Contains(got, `<span class="font-mono text-base font-semibold text-slate-900">br-lan.10</span>`) {
		t.Errorf("referenced interface should render mono at the MAC column's size and weight:\n%s", got)
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
		`colspan="7"`, "border-t border-slate-200", "verso-table-seam-row",
		"justify-start", "text-left", "group-hover:underline",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("seam missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hover:bg-slate-50") {
		t.Errorf("the table seam must not gain a hover background:\n%s", got)
	}
	if strings.Count(got, "<table") != 1 || strings.Count(got, "<thead>") != 1 {
		t.Errorf("the seam rows must share the original table and column head:\n%s", got)
	}
	if strings.Contains(render(t, r, redirectsTable()), "<details") {
		t.Error("a table without a seam should render no details element")
	}
}

// TestRenderTableEmpty: a listing with nothing in it drops its column heads —
// headings describe data, and over none they are chrome — and states the absence
// in one quiet full-width row on the wash. The plugin's own sentence wins; a
// listing that states none gets the shell's.
func TestRenderTableEmpty(t *testing.T) {
	r := newRenderer(t)
	bare := &Table{
		Columns: []TableColumn{{Label: "Type", Kind: "keyword"}, {Label: "Name", Kind: "mono"}},
	}
	got := render(t, r, bare)
	for _, want := range []string{
		`colspan="2"`, "bg-slate-50", "text-center", "text-slate-500",
		"border-b border-slate-200", // one hairline keeps the section's footprint
		">Nothing here yet<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty table missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"<thead", ">Type<", ">Name<"} {
		if strings.Contains(got, absent) {
			t.Errorf("empty table must draw no column head, found %q:\n%s", absent, got)
		}
	}

	bare.EmptyText = "No extra names yet — reserved devices already answer by name."
	if own := render(t, r, bare); !strings.Contains(own, bare.EmptyText) {
		t.Errorf("the listing's own sentence should carry the empty state:\n%s", own)
	}

	// Folded rows are rows: a seam holding some keeps the listing a listing.
	seamed := &Table{
		Columns: []TableColumn{{Label: "Type", Kind: "keyword"}},
		Seam:    &TableSeam{Summary: "stock entries", Rows: []TableRow{{Cells: []TableCell{{Text: "A"}}}}},
	}
	if got := render(t, r, seamed); !strings.Contains(got, "<thead") || strings.Contains(got, "Nothing here yet") {
		t.Errorf("a table whose rows are all folded is not empty:\n%s", got)
	}
}

// TestRenderEmptyTableDoesNotDrag: there is no order to state over no rows, so an
// empty listing draws neither handles nor the hidden order form the capsule would
// otherwise count.
func TestRenderEmptyTableDoesNotDrag(t *testing.T) {
	r := newRenderer(t)
	table := &Table{
		ReorderConfig: "firewall",
		Columns:       []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
	}
	got := render(t, r, table)
	for _, absent := range []string{"data-verso-reorder-table", "data-verso-page-form", ReorderConfigField} {
		if strings.Contains(got, absent) {
			t.Errorf("empty listing still carries %q:\n%s", absent, got)
		}
	}
	if n := PageFormCount(table); n != 0 {
		t.Errorf("PageFormCount = %d over an empty listing, want 0", n)
	}
}

// TestDecodeTableEmptyText pins the wire name a listing states its nothing under.
func TestDecodeTableEmptyText(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","columns":[{"label":"Server"}],"rows":[],` +
		`"empty_text":"Lookups follow what the internet connection suggested."}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	table, ok := w.(*Table)
	if !ok {
		t.Fatalf("decoded %T, want *Table", w)
	}
	if table.EmptyText != "Lookups follow what the internet connection suggested." {
		t.Errorf("EmptyText = %q", table.EmptyText)
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
	if err := r.RenderWithToken(&b, tbl, "tok123", "", nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := b.String()
	for _, want := range []string{
		`x-data="modal"`, `@click="show"`, ">Details<", // opens from the trailing link
		"dark:text-sky-400 dark:hover:text-sky-300", // stays legible on dark surfaces
		"x-teleport", "Edit redirect — Force-DNS-to-AdGuard-guest",
		"Save changes", `value="tok123"`, // the drawer's form carries the CSRF token
		"dark:bg-black/60",
		`verso-drawer-scrollbar absolute inset-y-0`,
		`<header class="flex shrink-0 items-center justify-between px-6 py-4">`, // the shared drawer panel's header
		"space-y-6 overflow-x-hidden px-6 pt-2 pb-6",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("row drawer missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		`@click="showFromRow"`, // the row itself is not clickable
		"m9 18 6-6-6-6",        // no row chevron — the "Details" link is the affordance
		`<header class="flex items-center justify-between border-b`,
	} {
		if strings.Contains(got, absent) {
			t.Errorf("inert row must not contain %q:\n%s", absent, got)
		}
	}
	if strings.Count(got, `x-data="modal"`) != 1 {
		t.Errorf("only drawer rows should host a modal scope:\n%s", got)
	}
}

func TestRenderTableRowDrawerCanHideVisibleTitle(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{Title: "Edit redirect", HideTitle: true}
	got := render(t, r, tbl)
	for _, want := range []string{`<h3 class="sr-only">Edit redirect</h3>`, "absolute top-5 right-5", "pt-6", `aria-label="Close"`} {
		if !strings.Contains(got, want) {
			t.Errorf("titleless drawer missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<h3 class="text-base font-medium text-slate-900">Edit redirect</h3>`) {
		t.Errorf("hidden drawer title must not remain visible:\n%s", got)
	}
}

// TestRenderTableRowDrawerOpen: a drawer the plugin sent back open renders its
// row's modal scope with data-open, which the shell's modal component reads on
// init — how a refused submission comes back with the failed drawer in front of
// the operator rather than closed.
func TestRenderTableRowDrawerOpen(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{Title: "Edit redirect", Open: true}
	got := render(t, r, tbl)
	if !strings.Contains(got, `<tr x-data="modal" data-open="true"`) {
		t.Errorf("open drawer row missing data-open:\n%s", got)
	}

	tbl.Rows[0].Drawer.Open = false
	closed := render(t, r, tbl)
	if strings.Contains(closed, "data-open") {
		t.Errorf("a closed drawer must claim nothing about opening:\n%s", closed)
	}
}

func TestRenderTableCustomDrawerAction(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.DrawerLabel = "Edit"
	tbl.DrawerIcon = "pencil"
	tbl.Rows[0].Drawer = &RowDrawer{Title: "Edit rule"}
	got := render(t, r, tbl)
	for _, want := range []string{"gap-1.5", lucideIcons["pencil"], ">Edit</button>"} {
		if !strings.Contains(got, want) {
			t.Errorf("custom drawer action missing %q:\n%s", want, got)
		}
	}
}

// TestDecodeTableRowDrawer: the drawer's children decode recursively, and an
// unknown child fails loudly.
func TestDecodeTableRowDrawer(t *testing.T) {
	w, err := Decode([]byte(`{
		"type": "table",
		"columns": [{"label":"A"}],
		"rows": [{"id":"r1","group":{"label":"WAN → Router","chain":"input_wan","count":2},"cells":[{"text":"1"}],
			"drawer": {"title":"Edit","hide_title":true,"open":true,"children":[{"type":"text","markdown":"body"}]}}]
	}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tb := w.(*Table)
	if tb.Rows[0].Drawer == nil || tb.Rows[0].Drawer.Title != "Edit" || !tb.Rows[0].Drawer.HideTitle || !tb.Rows[0].Drawer.Open || len(tb.Rows[0].Drawer.Children) != 1 {
		t.Errorf("row drawer not decoded: %+v", tb.Rows[0].Drawer)
	}
	if tb.Rows[0].Group == nil || tb.Rows[0].Group.Chain != "input_wan" || tb.Rows[0].Group.Count != 2 {
		t.Errorf("row group not decoded: %+v", tb.Rows[0].Group)
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

// TestRenderTableDisabledRowButton: a Disabled cell button keeps the compact
// row-button anatomy but is a real <button disabled> — muted ink, no hover, no
// press, and no form, modal, or drawer behind it.
func TestRenderTableDisabledRowButton(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Style:   "flat",
		Columns: []TableColumn{{Label: "Zone", Kind: "name"}, {Kind: "pill"}},
		Rows: []TableRow{{ID: "lan", Cells: []TableCell{
			{Text: "lan"},
			{Button: "Edit", Disabled: true},
		}}},
	})
	for _, want := range []string{
		`<button type="button" disabled`, ">Edit</button>",
		"inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium", // the shared row-button anatomy
		"cursor-not-allowed", "text-slate-400", "opacity-70",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("disabled row button missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"<form", `@click="show"`, `name="_action"`, `x-data="modal"`,
		"hover:border-slate-400", "active:translate-y-px", // an action nobody can take offers no feedback
	} {
		if strings.Contains(got, absent) {
			t.Errorf("disabled row button must not contain %q:\n%s", absent, got)
		}
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
		"columns": [{"kind":"toggle"},{"label":"From","kind":"endpoint"},{"label":"Hits","kind":"num"},{"kind":"pill"}],
		"rows": [{"id":"r1","key":"rule:r1","cells":[
			{"on":true,"name":"r1"},
			{"endpoints":[{"kind":"zone","label":"guest"},{"kind":"device","label":"10.0.0.30"}]},
			{"text":"28","key":"hits:r1"},
			{"button":"Edit","disabled":true}
		]}]
	}`))
	if err != nil {
		t.Fatalf("decode table: %v", err)
	}
	tb, ok := w.(*Table)
	if !ok {
		t.Fatalf("decoded %T, want *Table", w)
	}
	if len(tb.Columns) != 4 || tb.Columns[1].Kind != "endpoint" {
		t.Errorf("columns not decoded: %+v", tb.Columns)
	}
	if tb.Rows[0].ID != "r1" || len(tb.Rows[0].Cells[1].Endpoints) != 2 {
		t.Errorf("rows not decoded: %+v", tb.Rows)
	}
	if tb.Rows[0].Key != "rule:r1" || tb.Rows[0].Cells[2].Key != "hits:r1" {
		t.Errorf("live-update keys not decoded: %+v", tb.Rows[0])
	}
	if edit := tb.Rows[0].Cells[3]; edit.Button != "Edit" || !edit.Disabled {
		t.Errorf("disabled row button not decoded: %+v", edit)
	}
}

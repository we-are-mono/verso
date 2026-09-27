// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"slices"
	"strings"
	"testing"
)

// redirectsTable is the firewall's Redirects listing in miniature: one
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
		"text-xs font-medium tracking-[.08em] text-meta uppercase", // header treatment is the kicker, not a fill
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

// TestRenderTableFitColumns: fit columns squeeze to content through a colgroup
// width, so a huddle of related fact columns (a version pair and its arrow)
// sits together at the table's edge; a table with no fit columns draws no
// colgroup at all.
func TestRenderTableFitColumns(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{
			{Label: "Package", Kind: "name"},
			{Label: "Installed", Kind: "mono", Fit: true},
			{Kind: "keyword", Fit: true},
			{Label: "Available", Kind: "mono", Fit: true},
		},
		Rows: []TableRow{{Cells: []TableCell{{Text: "verso"}, {Text: "0.0.11"}, {Text: "→"}, {Text: "0.0.14"}}}},
	})
	if !strings.Contains(got, "<colgroup>") {
		t.Errorf("fit columns must draw a colgroup:\n%s", got)
	}
	if strings.Count(got, `<col class="w-px">`) != 3 {
		t.Errorf("each fit column carries the squeeze width:\n%s", got)
	}
	plain := render(t, r, redirectsTable())
	if strings.Contains(plain, "<colgroup") {
		t.Errorf("a table with no fit columns draws no colgroup:\n%s", plain)
	}
}

func TestRenderEmphasisedMonoCell(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Table{
		Columns: []TableColumn{{Label: "Address", Kind: "mono"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "10.0.0.232", Emphasis: true}}}},
	})
	// Emphasis is size alone: a mono value is 500 at every size.
	for _, want := range []string{"font-mono", "text-base", "font-medium", "10.0.0.232"} {
		if !strings.Contains(got, want) {
			t.Errorf("emphasised mono cell missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "font-semibold") {
		t.Errorf("emphasis must not thicken a mono value:\n%s", got)
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
		// Only the router fills its chip: it is the one end of a path that is
		// this device rather than something out on the network, and the action
		// colour is what says so. A chip, so the one chip box; sans, because
		// "router" names a thing rather than a value someone could type.
		chipBox + " border-denim-line bg-denim-soft text-denim-deep\">",
		// Every other end reads verbatim, in the mono reading size, inside the
		// same box drawn transparent so the column lines up.
		"border-transparent font-mono text-base font-medium tabular-nums text-ink",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("endpoints missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "rounded-full border") {
		t.Errorf("endpoints must not render as round pills inside a table:\n%s", got)
	}
	// The zone glyph (plain shield) and the device glyph (monitor) both appear.
	if !strings.Contains(got, lucideIcons["zone"]) {
		t.Errorf("zone endpoint should use the plain shield glyph:\n%s", got)
	}
	if !strings.Contains(got, lucideIcons["device"]) {
		t.Errorf("device endpoint should use the device glyph:\n%s", got)
	}
}

// TestRenderEndpointTypeIsFixedByKind: an endpoint reads the same wherever its
// column happens to sit. What it is — a zone, an address, the router — decides
// its type and its ground; where it is in the row decides nothing. Two listings
// that put From in different places therefore still line up.
func TestRenderEndpointTypeIsFixedByKind(t *testing.T) {
	r := newRenderer(t)
	leading := render(t, r, &Table{
		Columns: []TableColumn{{Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{Cells: []TableCell{{Endpoints: []TableEndpoint{
			{Kind: "zone", Label: "guest"},
		}}}}},
	})
	trailing := render(t, r, &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{Cells: []TableCell{{Text: "rule"}, {Endpoints: []TableEndpoint{
			{Kind: "zone", Label: "guest"},
		}}}}},
	})
	const want = "border-transparent font-mono text-base font-medium tabular-nums text-ink"
	if !strings.Contains(leading, want) || !strings.Contains(trailing, want) {
		t.Errorf("an endpoint's treatment must not depend on its column:\n%s\n%s", leading, trailing)
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
		// The handle is named by what the row is called, not its config
		// handle, and says the arrow keys move it: dragging is not the only way.
		`aria-label="Reorder guest"`, `aria-keyshortcuts="ArrowUp ArrowDown"`,
		"cursor-grab", "active:cursor-grabbing",
		// The grip rests at the step the eye passes over and darkens with the
		// whole row, not only under its own pointer.
		lucideIcons["list-chevrons-up-down"], "text-inert", "group-hover:text-glyph",
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
	if !strings.Contains(got, `class="w-6 border-b border-rule`) {
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
		ReorderConfig: "firewall",
		Columns:       []TableColumn{{Kind: "reorder"}, {Label: "From", Kind: "endpoint"}},
		Rows: []TableRow{{ID: "allow-dns", Group: &TableGroup{
			Key: "input_guest", Label: "Guest", To: "Router", Tally: "3 rules",
			AddLabel: "Add rule to Guest → Router", AddHref: "/x?open=new&src=guest",
		}, Cells: []TableCell{{}, {Endpoints: []TableEndpoint{{Kind: "zone", Label: "guest"}}}}}},
	})
	for _, want := range []string{
		`colspan="2"`,
		// The lane by its two ends, the arrow between them a glyph, then what
		// it amounts to after a faint dot.
		`text-sm font-semibold text-ink">Guest<span class="flex text-glyph">`, lucideIcons["arrow-right"], "</span>Router</span>",
		`<span class="text-faint">·</span><span class="text-meta">3 rules</span>`,
		// The lane spans the row, so it is both first and last child and takes
		// the wrapper's edge inset like every other cell. It is a row of the
		// listing's own 44px: a 28px line — the add's box — 8px above and
		// below, so a lane with no add is the same height as one with.
		`class="verso-table-group"`, `pt-2 pb-2 text-left leading-7 font-normal`, "[&_th:first-of-type]:pl-4",
		// The lane's add is a glyph at the band's right, its words on hover.
		`aria-label="Add rule to Guest → Router"`, lucideIcons["plus"], ">Add rule to Guest → Router</span>",
		// The handle is the drag's, not the band's.
		`data-verso-reorder-group="input_guest"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("grouped table missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "font-mono text-base leading-5 font-medium text-meta") {
		t.Errorf("a band with no verbatim string draws none:\n%s", got)
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
		"font-semibold text-ink",              // the identity column is emphasised ink, no icon
		"bg-green-soft text-green-deep",       // accept pill through the badge palette
		"bg-marigold-soft text-marigold-deep", // reject pill
		"bg-denim-soft text-denim-deep",       // NAT carries the info accent
		`<span class="text-inert"><span aria-hidden="true">—</span><span class="sr-only">None</span></span>`, // empty pill cell is a faint dash, said as "None"
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
	if !strings.Contains(got, `font-mono text-base font-medium text-ink">br-lan.10</span>`) {
		t.Errorf("referenced interface should render mono at the MAC column's size and weight:\n%s", got)
	}
	if !strings.Contains(got, ">family</span>") || !strings.Contains(got, lucideIcons["zone"]) {
		t.Errorf("referenced interface should carry the shared zone chip:\n%s", got)
	}
	// One chip box everywhere — 14px in a 16px line, 2px padding, 1px hairline —
	// so a reader never meets two heights of the same kind of thing; mono, so
	// one step heavier.
	if !strings.Contains(got, chipMonoBox+" border-rule bg-quiet text-meta") {
		t.Errorf("reference chips should carry the one chip treatment:\n%s", got)
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
		`colspan="7"`, "border-t border-rule", "verso-table-seam-row",
		"justify-start", "text-left", "group-hover:underline",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("seam missing %q:\n%s", want, got)
		}
	}
	// The fold's own control warms its ink, never its ground: a wash there would
	// read as one more row of the listing rather than as the seam between two.
	if seam := strings.Index(got, "verso-table-seam-control"); seam >= 0 {
		if control := got[seam:]; strings.Contains(control[:min(len(control), 600)], "hover:bg-") {
			t.Errorf("the table seam must not gain a hover background:\n%s", got)
		}
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
	// The canvas's one row, where the first row would sit: the row's own height
	// from padding, the listing's inset, meta words, left-aligned, a hairline.
	for _, want := range []string{
		`colspan="2" class="border-b border-rule px-4 py-2.5 text-left text-sm leading-6 text-meta group-last:border-b-0"`,
		">Nothing here yet<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty table missing %q:\n%s", want, got)
		}
	}
	for _, never := range []string{"bg-quiet", "text-center", "py-6"} {
		if strings.Contains(got, never) {
			t.Errorf("the empty row is a plain row — no %q:\n%s", never, got)
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
// empty listing draws neither handles nor the hidden order form a drop would
// otherwise post.
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
		"text-denim transition-colors hover:text-denim-deep", // the action colour, like every other link
		"x-teleport", "Edit redirect — Force-DNS-to-AdGuard-guest",
		"Save changes", `value="tok123"`, // the drawer's form carries the CSRF token
		"bg-ink/18",
		// The panel is a column that does not itself scroll: the nameplate and
		// the tab strip stay put and only the body moves under them, so what
		// the panel is about is still on screen at the bottom of a long form.
		`absolute inset-y-0 right-0 flex w-full`,
		"flex-col overflow-hidden border-l border-rule-strong",
		`<header class="flex h-13 flex-none items-center gap-4 border-b border-rule bg-quiet px-8">`, // the shared drawer panel's header
		"verso-drawer-scrollbar min-h-0 flex-1 space-y-6 overflow-x-hidden overflow-y-auto px-8",
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

func TestRenderTableRowDrawerKeepsLegacyHiddenTitleVisible(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{Title: "Edit redirect", HideTitle: true}
	got := render(t, r, tbl)
	for _, want := range []string{`<h2 class="min-w-0 truncate text-lg font-semibold tracking-tight text-body">Edit redirect</h2>`, "bg-quiet px-8", "pt-7", `aria-label="Close"`} {
		if !strings.Contains(got, want) {
			t.Errorf("drawer title band missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<h2 class="sr-only">Edit redirect</h2>`) {
		t.Errorf("legacy hide_title must not remove the title band:\n%s", got)
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
		"rows": [{"id":"r1","group":{"label":"WAN","to":"Router","chain":"input_wan","tally":"2 rules"},"cells":[{"text":"1"}],
			"drawer": {"title":"Edit","hide_title":true,"open":true,"children":[{"type":"text","markdown":"body"}]}}]
	}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tb := w.(*Table)
	if tb.Rows[0].Drawer == nil || tb.Rows[0].Drawer.Title != "Edit" || !tb.Rows[0].Drawer.HideTitle || !tb.Rows[0].Drawer.Open || len(tb.Rows[0].Drawer.Children) != 1 {
		t.Errorf("row drawer not decoded: %+v", tb.Rows[0].Drawer)
	}
	if tb.Rows[0].Group == nil || tb.Rows[0].Group.To != "Router" || tb.Rows[0].Group.Chain != "input_wan" || tb.Rows[0].Group.Tally != "2 rules" {
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
		">Connected devices<",  // header band title
		"12 online · 3 busy",   // header band detail
		">View all<",           // header band action
		`href="/devices"`,      // action link target
		">Details<",            // the trailing "more" affordance
		`@click="show"`,        // the Details link opens the drawer
		"border-b border-rule", // hairline rows
		// A cell is a 24px line inset 10px above and below — which is the
		// 44px row, arrived at rather than stated.
		"px-3.5 py-2.5 leading-6",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("flat table missing %q:\n%s", want, got)
		}
	}
	// The row itself is inert — no pointer, no click — so its values stay
	// selectable; the trailing "Details" button is the one thing that opens.
	if strings.Contains(got, `<tr class="group transition-colors hover:bg-quiet/50 cursor-pointer`) {
		t.Errorf("flat rows must not take the pointer:\n%s", got)
	}
	for _, absent := range []string{
		"odd:bg-quiet",         // stripes are retired in the flat style
		`@click="showFromRow"`, // the row itself is not clickable
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

// TestRenderTableDense: every row is inset from the table's edges so a hover
// tint and a band's fill, which run the full width, never touch a value. Dense
// drops the cells' own inset for a listing carrying many columns — the columns'
// widths space them — and keeps the edge inset; the rows keep their height
// either way, because density here is horizontal.
func TestRenderTableDense(t *testing.T) {
	r := newRenderer(t)
	columns := []TableColumn{{Label: "Name", Kind: "name"}}
	rows := []TableRow{{Cells: []TableCell{{Text: "SSH"}}}}

	roomy := render(t, r, &Table{Style: "flat", Columns: columns, Rows: rows})
	for _, want := range []string{"[&_td:first-of-type]:pl-4", "[&_th:last-of-type]:pr-4", "px-3.5 py-2.5 leading-6"} {
		if !strings.Contains(roomy, want) {
			t.Errorf("a listing's rows are inset from its edges, missing %q:\n%s", want, roomy)
		}
	}
	dense := render(t, r, &Table{Style: "flat", Dense: true, Columns: columns, Rows: rows})
	for _, want := range []string{"[&_td:first-of-type]:pl-4", "[&_th:last-of-type]:pr-4", `class="relative border-b border-rule py-2.5 leading-6 pr-4 align-top`} {
		if !strings.Contains(dense, want) {
			t.Errorf("a dense listing keeps the edge inset and drops the cells' own, missing %q:\n%s", want, dense)
		}
	}
	if strings.Contains(dense, "px-3.5") {
		t.Errorf("a dense listing's cells carry no inset of their own:\n%s", dense)
	}
	// Of-type, not child: a row carrying an entity panel's teleport template
	// after its last cell would otherwise miss the inset on that cell.
	if strings.Contains(roomy, ":last-child]") || strings.Contains(dense, ":last-child]") {
		t.Error("the edge inset must not depend on the last cell being the row's last child")
	}
	// Density is horizontal: a dense listing keeps the row height of a roomy one.
	if strings.Contains(dense, "[&_td]:py-1.5") {
		t.Errorf("dense should not tighten the row's vertical rhythm:\n%s", dense)
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
		"inline-flex h-7 items-center gap-1.5 rounded-xs border px-3 text-sm font-semibold", // the shared small-button anatomy
		"cursor-not-allowed", "text-faint", "opacity-70",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("disabled row button missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{
		"<form", `@click="show"`, `name="_action"`, `x-data="modal"`,
		"hover:border-denim", "verso-press", // an action nobody can take offers no feedback
	} {
		if strings.Contains(got, absent) {
			t.Errorf("disabled row button must not contain %q:\n%s", absent, got)
		}
	}
}

// TestRowButtonNamesItsRow: a row's labelled act says which row it acts on, so
// a list of buttons read out of the table ("Revoke, Revoke") is still a list
// of different acts. The visible label leads the name, as a voice user speaks it.
func TestRowButtonNamesItsRow(t *testing.T) {
	r := newRenderer(t)
	for name, cell := range map[string]TableCell{
		"confirmed": {Button: "Revoke", Action: "end:a", Confirm: "Signed out."},
		"direct":    {Button: "Revoke", Action: "end:a"},
		"disabled":  {Button: "Revoke", Disabled: true},
	} {
		got := render(t, r, &Table{
			Columns: []TableColumn{{Label: "Source", Kind: "name"}, {Kind: "pill"}},
			Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "10.0.0.229"}, cell}}},
		})
		if !strings.Contains(got, `aria-label="Revoke 10.0.0.229"`) {
			t.Errorf("%s: the row button must name its row:\n%s", name, got)
		}
	}
}

// TestRowsHangFromTheirFirstLine: every cell stands at the row's top, so when
// one cell spans two lines the others — the row's acts above all — stay level
// with its first line rather than floating to the middle of the block. A
// stacked cell holds its first line on the same 22px centre a one-line cell's
// 24px line has, and the row keeps its height.
func TestTableRowsHangFromTheirFirstLine(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Kind: "addr"}, {Kind: "actions"}},
		Rows: []TableRow{{ID: "k", Cells: []TableCell{{Text: "demo@laptop", Sub: "SHA256:x"}, {Actions: []TableRowAct{
			{Icon: "trash-2", Title: "Remove", Href: "/remove"},
		}}}}},
	})
	if strings.Count(got, "align-top") < 2 {
		t.Errorf("every cell stands at the row's top:\n%s", got)
	}
	if strings.Contains(got, "align-middle") {
		t.Errorf("no cell centres itself against the others:\n%s", got)
	}
	if !strings.Contains(got, "pt-3 pb-2") {
		t.Errorf("a stacked cell lifts its first line onto the one-line centre:\n%s", got)
	}
}

// TestRowActNamesItsRow: a row's icon act is named with its row too — a column
// of trash cans read out of the table is still a list of different keys. The
// tip a pointer rests on keeps the short word; the row is already in view. An
// inert mark (no destination) explains a state and keeps its own sentence.
func TestRowActNamesItsRow(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Kind: "name"}, {Kind: "actions"}},
		Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "demo@laptop"}, {Actions: []TableRowAct{
			{Icon: "trash-2", Title: "Remove", Href: "/remove?a"},
			{Icon: "pencil", Title: "Edit"},
		}}}}},
	})
	for _, want := range []string{`aria-label="Remove demo@laptop"`, `aria-label="Edit"`} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// the row is named by its identity, never by a placeholder dash that
	// happens to sit in an earlier column
	svc := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Order", Kind: "mono"}, {Label: "Service", Kind: "reference"}, {Kind: "actions"}},
		Rows: []TableRow{{ID: "fw", Cells: []TableCell{{Text: "—"}, {Text: "firewall"}, {Actions: []TableRowAct{
			{Icon: "rotate-cw", Title: "Restart", Name: "_action", Value: "restart:firewall"},
		}}}}},
	})
	if !strings.Contains(svc, `aria-label="Restart firewall"`) {
		t.Errorf("want the row named by its service:\n%s", svc)
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
		"size-1.5 shrink-0 rounded-[1px] bg-crimson", `>End session<`, `>Cancel<`, // the alarm's mark: the square at full chroma
		"border-crimson bg-crimson text-white",        // the destructive act
		"border-rule-strong bg-transparent text-meta", // Cancel is the ordinary quiet button
		"verso-press",
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

// streamTable is the firewall's live activity listing in miniature: the
// columns the verdict stream arrives under, declared live and empty, because a
// stream table's rows come after the render.
func streamTable() *Table {
	return &Table{
		Dense:  true,
		Stream: &TableStream{Source: StreamSourceFirewallLog},
		Columns: []TableColumn{
			{Label: "When", Kind: "runtime"},
			{Label: "Count", Kind: "num"},
			{Label: "Verdict", Kind: "pill"},
			{Label: "From", Kind: "endpoint"},
			{Label: "Source", Kind: "mono"},
			{Label: "To", Kind: "endpoint"},
			{Label: "Protocol", Kind: "keyword"},
			{Label: "Port", Kind: "mono"},
			{Label: "Rule", Kind: "link"},
		},
		EmptyText: "Waiting for the first logged event…",
	}
}

// TestRenderStreamTableWiresItsSource: a live listing states the source and the
// ring on the table itself — that pair is the whole contract the client reads.
func TestRenderStreamTableWiresItsSource(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, streamTable())
	for _, want := range []string{
		`data-verso-stream="firewall-log"`,
		`data-verso-stream-ring="200"`, // the default ring, since none was declared
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stream table missing %q:\n%s", want, got)
		}
	}
}

// TestRenderStreamTableKeepsItsHeadWhileWaiting: an ordinary empty table drops
// its column heads (chrome over no data); a live one keeps them, because they
// name what is about to arrive and the first event must not shift the layout.
// The quiet sentence row is marked so that first event can replace it.
func TestRenderStreamTableKeepsItsHeadWhileWaiting(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, streamTable())
	for _, want := range []string{
		"<thead>", ">Verdict<", ">Rule<",
		"data-verso-stream-empty",
		"Waiting for the first logged event…",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("waiting stream table missing %q:\n%s", want, got)
		}
	}
	// The same table without the stream is the ordinary empty listing.
	still := streamTable()
	still.Stream = nil
	if got := render(t, r, still); strings.Contains(got, "<thead>") {
		t.Errorf("a still empty table must drop its heads:\n%s", got)
	}
}

// TestStreamTableDefaultsItsWaitingSentence: a live listing that declares no
// empty text says it is waiting, not that there is nothing — the difference is
// the whole honesty of a stream that has not seen its first event.
func TestStreamTableDefaultsItsWaitingSentence(t *testing.T) {
	r := newRenderer(t)
	tb := streamTable()
	tb.EmptyText = ""
	got := render(t, r, tb)
	if !strings.Contains(got, "Waiting for the first event…") {
		t.Errorf("stream table missing its waiting sentence:\n%s", got)
	}
	if strings.Contains(got, "Nothing here yet") {
		t.Errorf("a live listing must not claim there is nothing:\n%s", got)
	}
}

// TestStreamRingIsClamped: the ring is a browser budget, so a plugin's number
// is held to sane bounds rather than trusted.
func TestStreamRingIsClamped(t *testing.T) {
	for _, tc := range []struct{ declared, want int }{
		{0, StreamRingDefault},
		{-5, StreamRingDefault},
		{1, StreamRingMin},
		{50, 50},
		{100000, StreamRingMax},
	} {
		s := &TableStream{Source: StreamSourceFirewallLog, Ring: tc.declared}
		if got := s.ring(); got != tc.want {
			t.Errorf("ring(%d) = %d, want %d", tc.declared, got, tc.want)
		}
	}
}

// TestUnknownStreamSourceRendersStill: the source set is closed. A table naming
// a feed nobody wrote renders as an ordinary listing — no wiring, no dead
// EventSource, and no head over an empty body either.
func TestUnknownStreamSourceRendersStill(t *testing.T) {
	r := newRenderer(t)
	tb := streamTable()
	tb.Stream = &TableStream{Source: "syslog"}
	got := render(t, r, tb)
	if strings.Contains(got, "data-verso-stream") {
		t.Errorf("an unknown source must not be wired:\n%s", got)
	}
	if strings.Contains(got, "<thead>") {
		t.Errorf("an unwired table is a still empty table:\n%s", got)
	}
	if !StreamSourceKnown(StreamSourceFirewallLog) {
		t.Error("the firewall log is a source the shell serves")
	}
	if StreamSourceKnown("syslog") {
		t.Error("syslog is not in the closed set")
	}
}

// TestDecodeTableStream: the wire shape of a live listing round-trips.
func TestDecodeTableStream(t *testing.T) {
	w, err := Decode([]byte(`{
		"type": "table",
		"stream": {"source":"firewall-log","ring":120},
		"columns": [{"label":"When","kind":"runtime"}],
		"rows": []
	}`))
	if err != nil {
		t.Fatalf("decode stream table: %v", err)
	}
	tb, ok := w.(*Table)
	if !ok {
		t.Fatalf("decoded %T, want *Table", w)
	}
	if tb.Stream == nil || tb.Stream.Source != StreamSourceFirewallLog || tb.Stream.Ring != 120 {
		t.Fatalf("stream not decoded: %+v", tb.Stream)
	}
	if !tb.streaming() || tb.streamRing() != 120 {
		t.Errorf("decoded stream table is not live: streaming=%v ring=%d", tb.streaming(), tb.streamRing())
	}
}

// TestDecodeTableRowCarriesEveryField: a row's own fields survive the wire. The
// decoder names each one explicitly, so a field it forgets is dropped in
// silence — the row still renders, just without the thing the plugin asked for.
// This is the pin that makes that failure loud.
func TestDecodeTableRowCarriesEveryField(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","columns":[{"kind":"name"}],"rows":[{
		"id":"allow_ping","key":"rule:allow_ping","muted":true,
		"tags":["ipv4","ipv6"],"facet":{"network":"guest"},
		"entity":{"kind":"device","id":"aa:bb:cc:dd:ee:ff"},
		"cells":[{"text":"Allow-Ping"}]}]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	row := w.(*Table).Rows[0]
	if row.ID != "allow_ping" || row.Key != "rule:allow_ping" || !row.Muted {
		t.Errorf("identity or mute lost in decode: %#v", row)
	}
	if strings.Join(row.Tags, " ") != "ipv4 ipv6" {
		t.Errorf("tags lost in decode: %#v", row.Tags)
	}
	if row.Facet["network"] != "guest" {
		t.Errorf("facet lost in decode: %#v", row.Facet)
	}
	if row.Entity == nil || row.Entity.ID != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("entity lost in decode: %#v", row.Entity)
	}
	// And they reach the markup the bar narrows by.
	got := render(t, newRenderer(t), w)
	for _, want := range []string{`data-verso-tags="ipv4 ipv6"`, `data-verso-facet-network="guest"`, "text-meta"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded row missing %q in:\n%s", want, got)
		}
	}
}

// TestDecodeRowDrawerCarriesEveryField: the row drawer's own fields survive the
// wire too, and they are decoded by a shape of their own nested inside the
// row's — so it is a second list that has to stay complete, with the same
// silent failure when it does not.
func TestDecodeRowDrawerCarriesEveryField(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","columns":[{"kind":"name"}],"rows":[{
		"id":"allow_ping","cells":[{"text":"Allow-Ping"}],
		"drawer":{"title":"Allow-Ping","verbatim":true,"sub":"from the installer",
			"chain":"input_wan","tag":"guest","size":"wide","open":true,
			"verdict":{"type":"badge","text":"accept","variant":"success"},
			"lede":["Rule 1 of 22","0 matches since boot"],
			"tabs":[{"label":"Match","state":"5 conditions","href":"/x?tab=match","active":true},
				{"label":"Action","state":"accept","href":"/x?tab=action"}],
			"children":[{"type":"form","submit":"Save","fields":[{"type":"field","name":"n","label":"Name"}]}]}}]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	d := w.(*Table).Rows[0].Drawer
	if d == nil {
		t.Fatal("drawer lost in decode")
	}
	if !d.Verbatim || d.Sub != "from the installer" || d.Chain != "input_wan" || d.Tag != "guest" {
		t.Errorf("nameplate lost in decode: %#v", d)
	}
	if d.Verdict == nil || d.Verdict.Text != "accept" || d.Verdict.Variant != "success" {
		t.Errorf("verdict lost in decode: %#v", d.Verdict)
	}
	if strings.Join(d.Lede, "|") != "Rule 1 of 22|0 matches since boot" {
		t.Errorf("lede lost in decode: %#v", d.Lede)
	}
	if len(d.Tabs) != 2 || d.Tabs[0].State != "5 conditions" || !d.Tabs[0].Active || d.Tabs[1].Href != "/x?tab=action" {
		t.Errorf("tabs lost in decode: %#v", d.Tabs)
	}
}

// Legacy decoration fields must not reintroduce subtitles or badges into the
// title band. Tabs and the form remain available below it.
func TestRenderRowDrawerTitleBand(t *testing.T) {
	r := newRenderer(t)
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{
		Title: "Allow-DHCP-Renew", Chain: "input_wan", Size: "wide",
		Verdict: &Badge{Text: "accept", Variant: "success"},
		Lede:    []string{"Rule 1 of 22", "0 matches since boot"},
		Tabs: []DrawerTab{
			{Label: "Match", State: "5 conditions", Href: "/plugins/firewall/?open=r1&tab=match", Active: true},
			{Label: "Action", State: "accept", Href: "/plugins/firewall/?open=r1&tab=action"},
		},
		Children: []Widget{&Form{Submit: "Save", Fields: []Widget{&Field{Name: "name", Label: "Name"}}}},
	}
	got := render(t, r, tbl)
	for _, want := range []string{
		`<header class="flex h-13 flex-none items-center gap-4 border-b border-rule bg-quiet px-8">`,
		`<h2 class="min-w-0 truncate text-lg font-semibold tracking-tight text-body">Allow-DHCP-Renew</h2>`,
		// The strip: the tab in force carries the action colour under it and in
		// its chip; the rest stay quiet.
		`<nav class="flex h-13 flex-none gap-6 overflow-x-auto border-b border-rule px-8"`,
		`aria-current="page"`,
		"shadow-[inset_0_-2px_0_var(--color-denim-deep)]",
		"border-denim-line bg-denim-soft text-denim-deep\">5 conditions",
		"border-rule bg-quiet text-meta\">accept",
		// A tabbed panel's body starts at the canvas's 28px under the strip.
		"overflow-y-auto px-8 pt-7 pb-8",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("drawer nameplate missing %q:\n%s", want, got)
		}
	}
	for _, stale := range []string{"input_wan", "Rule 1 of 22", "0 matches since boot", "border-green-line bg-green-soft text-green-deep"} {
		if strings.Contains(got, stale) {
			t.Errorf("drawer retained legacy header decoration %q", stale)
		}
	}
}

// TestRenderRowDrawerWithoutTabs: a panel with one thing to say draws no strip —
// a row of headings that decides nothing is chrome. The body starts at the
// same inset below the title band as it would below a tab strip.
func TestRenderRowDrawerWithoutTabs(t *testing.T) {
	tbl := redirectsTable()
	tbl.Rows[0].Drawer = &RowDrawer{Title: "Allow-DHCP-Renew", Children: []Widget{&Callout{Body: "Nothing to choose between."}}}
	got := render(t, newRenderer(t), tbl)
	if strings.Contains(got, `aria-label="Sections"`) {
		t.Errorf("a panel with no tabs must draw no strip:\n%s", got)
	}
	if !strings.Contains(got, "overflow-y-auto px-8 pt-7 pb-8") {
		t.Errorf("untabbed panel lost its own top:\n%s", got)
	}
}

// TestRowWithItsOwnDoorDrawsNoDetailsLink: a listing whose rows already say how
// they are entered — a linked name, the acts at the trailing edge — gets no
// "Details" link and no extra column for one. The shell's affordance is for a
// row that carries nothing; beside a row that carries its own it is a second
// door next to a door, and the column it needs shifts every other cell along.
// TestRowActPostsItselfAndTheRowNamesItsSection: an act that posts is a form
// of its own, marked so the shell's script can post it where it stands, and the
// row names the section it is so the answer's row can be matched back to it. A
// row with no id names nothing, and an act that leads somewhere is a link.
func TestRowActPostsItselfAndTheRowNamesItsSection(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Kind: "actions"}},
		Rows: []TableRow{
			{ID: "allow_ping", Cells: []TableCell{{Text: "Allow-Ping"}, {Actions: []TableRowAct{
				{Icon: "power-off", Title: "Disable", Name: "allow_ping", Value: "off"},
				{Icon: "square-pen", Title: "Edit", Href: "/x?open=allow_ping"},
			}}}},
			{Cells: []TableCell{{Text: "Nameless"}, {}}},
		},
	})
	for _, want := range []string{
		`<tr class="group transition-colors hover:bg-quiet/50" data-verso-row-id="allow_ping">`,
		`<form method="post" data-verso-act class="contents">`,
		`<input type="hidden" name="allow_ping" value="off">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("row act missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "data-verso-act") != 1 {
		t.Errorf("only the act that posts is marked:\n%s", got)
	}
	if strings.Count(got, "data-verso-row-id") != 1 {
		t.Errorf("a row with no id names no section:\n%s", got)
	}
}

// TestEveryCellGivesUpTheLastRowsHairline: whatever its kind, a cell on the
// last row draws no hairline of its own — the table's closing line, or the
// section's rule when the table ends its section, is the only line under it.
func TestEveryCellGivesUpTheLastRowsHairline(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "File", Kind: "path"}, {Label: "Options", Kind: "count"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "ads.conf", Sub: "/etc/dnsmasq.d/"}, {Text: "2", Sub: "options"}}}},
	})
	for _, td := range strings.Split(got, "<td")[1:] {
		td = td[:strings.Index(td, ">")]
		if strings.Contains(td, "border-b") && !strings.Contains(td, "group-last:border-b-0") {
			t.Errorf("a cell keeps its hairline on the last row: <td%s>", td)
		}
	}
}

// TestAGroupWithNothingInItSaysSo: a row that carries only its group is the
// band alone — its name, its count, its add — and when nothing else is listed
// the table says its empty sentence under that band, one row across every
// column in words, never a fake row in a value column. Heads describe data, so
// over none there are none; and the sentence gives up its own hairline to
// whatever closes the table.
func TestAGroupWithNothingInItSaysSo(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns:   []TableColumn{{Label: "File", Kind: "path"}, {Label: "Options", Kind: "count"}, {Kind: "actions"}},
		EmptyText: "No custom option files",
		Rows:      []TableRow{{Group: &TableGroup{Label: "Files", Chain: "0", AddLabel: "New file", AddText: "New file", AddHref: "/x/files/new"}}},
	})
	for _, want := range []string{
		"verso-table-group", ">Files<", ">0<", ">New file<",
		`<tr class="group"><td colspan="3" class="border-b border-rule px-4 py-2.5 text-left text-sm leading-6 text-meta group-last:border-b-0">No custom option files</td></tr>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<thead") || strings.Contains(got, "data-verso-row-id") || strings.Count(got, "<td") != 1 {
		t.Errorf("a band with nothing under it draws no heads and no empty cells:\n%s", got)
	}
}

// TestCellDetailStandsUnderItsValue: a value that needs a line of words to say
// what it is (the browser behind an address) or since when (a session's
// start) carries them on a second line under it, in the secondary step, and the
// cell takes the stacked row's padding. A tag rides the detail line.
func TestCellDetailStandsUnderItsValue(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Source", Kind: "reference"}, {Label: "Last seen"}},
		Rows: []TableRow{{ID: "s1", Cells: []TableCell{
			{Text: "fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b", Detail: "Safari · iPhone", Tag: "this browser", TagVariant: "success"},
			{Text: "Now", Detail: "since today, 16:43"},
		}}},
	})
	for _, want := range []string{
		`<span class="flex items-center gap-x-2 text-sm leading-5 whitespace-nowrap text-meta"><span>Safari · iPhone</span>`,
		`<span class="flex items-center gap-x-2 text-sm leading-5 whitespace-nowrap text-meta"><span>since today, 16:43</span>`,
		`>fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b</span>`,
		">this browser</span>",
		// the 22px chip overhangs the 20px detail line by a pixel either side
		// rather than making the line taller
		chipBox + " -my-px",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// Even air around the stack: 12px above and below, so its first line keeps
	// the 22px centre of a one-line cell and of the row's acts.
	for _, want := range []string{
		`relative border-b border-rule px-3.5 py-3 align-top`,
		`border-b border-rule px-3.5 py-3 align-top group-last:border-b-0"><div class="leading-5`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a stacked cell sits evenly in its row, want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "text-faint\">·</span>") {
		t.Errorf("a detail is its own line, not an inline qualifier:\n%s", got)
	}
}

// chipBox is the one chip every citation wears, the Interfaces page's: 14px
// regular words on a 16px line, 2px above and below, inside a 1px border.
const chipBox = "inline-flex items-center gap-1.5 rounded-xs border px-1.5 py-0.5 leading-4 whitespace-nowrap text-sm font-normal"

// chipMonoBox is the same box for a chip whose words the machine wrote: mono,
// one step heavier (500), because mono at 400 reads a size smaller than the
// sans beside it.
const chipMonoBox = "inline-flex items-center gap-1.5 rounded-xs border px-1.5 py-0.5 leading-4 whitespace-nowrap text-sm font-medium font-mono"

// TestTableTagAlwaysWearsItsHairline: a tag is a chip — the same box as every
// other chip, with the hairline of its own hue — with or without an icon.
func TestTableTagAlwaysWearsItsHairline(t *testing.T) {
	for variant, tone := range map[string]string{
		"":        "border-rule bg-quiet text-meta",
		"info":    "border-denim-line bg-denim-soft text-denim-deep",
		"warning": "border-marigold-line bg-marigold-soft text-marigold-deep",
		"success": "border-green-line bg-green-soft text-green-deep",
		"danger":  "border-crimson-line bg-crimson-soft text-crimson-deep",
	} {
		want := chipBox + " " + tone
		for _, icon := range []string{"", "globe"} {
			got := render(t, newRenderer(t), &Table{
				Columns: []TableColumn{{Label: "Source", Kind: "reference"}},
				Rows:    []TableRow{{ID: "r", Cells: []TableCell{{Text: "10.0.0.2", Tag: "this browser", TagVariant: variant, TagIcon: icon}}}},
			})
			if !strings.Contains(got, want) {
				t.Errorf("tag %q (icon %q) wants %q in:\n%s", variant, icon, want, got)
			}
		}
	}
}

// TestPostingActAsksFirstWhenItsCellDoes: an icon act that ends something
// asks before it posts, in the same alert the labelled row act uses; its
// danger button is named with the act.
func TestPostingActAsksFirstWhenItsCellDoes(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Source", Kind: "reference"}, {Kind: "actions"}},
		Rows: []TableRow{{ID: "s2", Cells: []TableCell{
			{Text: "10.0.10.117"},
			{Actions: []TableRowAct{{Icon: "log-out", Title: "Revoke session", Name: "_action", Value: "end-session:s2"}},
				Confirm: "Whoever holds it is signed out.", ConfirmTitle: "Revoke this session?"},
		}}},
	})
	for _, want := range []string{
		`x-data="modal"`, `role="alertdialog"`, ">Revoke this session?</h2>",
		`name="_action" value="end-session:s2"`,
		`aria-label="Revoke session 10.0.10.117"`,
		">Revoke session</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "data-verso-act") {
		t.Errorf("an act that asks first must not post where it stands:\n%s", got)
	}
}

func TestConfirmationBelongsToItsPostingAction(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","columns":[{"label":"Network","kind":"name"},{"kind":"actions"}],"rows":[{"cells":[{"text":"Guest <wifi>"},{"actions":[{"icon":"power","title":"Disable","name":"guest","value":"off"},{"icon":"trash-2","title":"Remove network","name":"_remove","value":"guest","confirm_title":"Remove network “%s”?","confirm":"Applied later."}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	r := newRenderer(t)
	tr := fakeCatalog(map[string]string{"Remove network “%s”?": "Odstranim omrežje »%s«?", "Applied later.": "Velja po uveljavitvi."})
	var out strings.Builder
	if err := r.RenderWithToken(&out, w, "csrf-example", "", tr); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Count(got, `role="alertdialog"`) != 1 || strings.Count(got, `data-verso-act`) != 1 {
		t.Fatalf("only removal asks first; the power act remains direct:\n%s", got)
	}
	for _, want := range []string{`Odstranim omrežje »Guest &lt;wifi&gt;«?`, `Velja po uveljavitvi.`, `name="_remove" value="guest"`, `value="csrf-example"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRowWithItsOwnDoorDrawsNoDetailsLink(t *testing.T) {
	open := func(cells []TableCell) string {
		return render(t, newRenderer(t), &Table{
			Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Kind: "actions"}},
			Rows: []TableRow{{ID: "r1", Cells: cells, Drawer: &RowDrawer{
				Title: "Allow-Ping", Children: []Widget{&Callout{Body: "."}},
			}}},
		})
	}
	// The rule listing's shape: the name is the door and the row ends in acts.
	withDoor := open([]TableCell{
		{Text: "Allow-Ping", Href: "/plugins/firewall?open=r1"},
		{Actions: []TableRowAct{{Icon: "square-pen", Title: "Edit", Href: "/plugins/firewall?open=r1"}}},
	})
	if strings.Contains(withDoor, ">Details<") {
		t.Errorf("a row with its own door must not grow a Details link:\n%s", withDoor)
	}
	// A row carrying nothing but text still needs the shell to offer a way in.
	bare := open([]TableCell{{Text: "htop"}, {Text: "3.5.1"}})
	if !strings.Contains(bare, ">Details<") {
		t.Errorf("a row with no door of its own still needs one:\n%s", bare)
	}
}

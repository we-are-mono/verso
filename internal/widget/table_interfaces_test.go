// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inventoryColumns are the inventory's columns by kind: the template finds a
// row's state and acts by what their columns are, never by where they stand.
func inventoryColumns() []TableColumn {
	return []TableColumn{
		{Label: "Device · network", Kind: "reference"}, {Label: "Type", Kind: "keyword"},
		{Label: "Address", Kind: "mono"}, {Label: "State", Kind: "status"}, {Kind: "actions"},
	}
}

func TestInterfaceTreeKeepsBranchesThroughDescendants(t *testing.T) {
	table := &Table{Style: "interfaces", Columns: []TableColumn{{Kind: "text"}}, Rows: []TableRow{
		{ID: "eth4", Cells: []TableCell{{Text: "eth4", LeadIcon: "physical"}}},
		{ID: "eth4.3900", Depth: 1},
		{ID: "pppoe-wan", Depth: 2},
		{ID: "eth4.4000", Depth: 1},
		{ID: "br-lan"},
		{ID: "lan0", Depth: 1},
		{ID: "wan0"},
	}}
	r := newRenderer(t)
	rows, err := table.rowViews(r, "", table.Rows, false)
	if err != nil {
		t.Fatal(err)
	}
	// The physical marker is centered on its own stem, at x=8; the tree is
	// drawn on the row's two 20px cells, its branches on the 20px middle.
	if rows[0].TreeNodeX != 4 || rows[0].TreeNodeCenter != 8 || !strings.Contains(rows[0].TreePath, "M8 20V40") {
		t.Fatalf("root marker and branch disagree: %+v", rows[0])
	}
	for _, want := range []struct {
		index              int
		path, continuation string
	}{
		{1, "M8 0V40", "M8 0V1M32 0V1"}, // Both the ancestor and child continue below this row.
		{2, "M32 0V20", "M8 0V1"},       // PPP ends, but the next VLAN still needs the outer stem.
		{3, "M8 0V20", ""},              // Last VLAN ends the physical port's branch.
		{4, "M8 20V40", "M8 0V1"},       // The next root starts a separate bridge branch.
		{5, "M8 0V20", ""},              // Only lan0 belongs to that bridge.
		{6, "M8 20H66", ""},             // WAN remains independent.
	} {
		got := rows[want.index]
		if !strings.Contains(got.TreePath, want.path) || got.TreeContinuation != want.continuation {
			t.Errorf("%s: path=%q continuation=%q", got.ID, got.TreePath, got.TreeContinuation)
		}
	}
	got := render(t, r, table)
	// A wrapped row hangs from its first line: the node and its branch stay on
	// the first two cells, 40px, and only the trunks that run on to later rows
	// stretch down the rest of the row, without changing their stroke.
	// The tree starts 11px in, where every listing's first content does, the
	// first column's head over the names after it.
	for _, want := range []string{
		`<table class="verso-table" data-verso-dense data-verso-tree>`,
		`<td class="verso-table__cell verso-table__tree">`,
		`class="verso-table__branch"`, `class="verso-table__trunk"`,
		`preserveAspectRatio="none"`, `vector-effect="non-scaling-stroke"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	declares(t, "table", ".verso-table[data-verso-tree] .verso-table__tree", "position: relative", "padding: 0 0 0 calc(var(--spacing) * 2.75)")
	declares(t, "table", ".verso-table__branch", "top: 0", "height: calc(var(--spacing) * 10)")
	declares(t, "table", ".verso-table__trunk", "top: calc(var(--spacing) * 10)", "bottom: 0")
	// The first head stands over the names: the table's 11px edge, the tree's
	// 66px and the name's 12px lead-in.
	declares(t, "table", ".verso-table[data-verso-tree] .verso-table__head:first-of-type", "padding-inline-start: calc(var(--spacing) * 22.25)")
	// Every cell of a row stands at its top: each is the table block's, and
	// the block stands every cell of its table at the row's top.
	if n := strings.Count(got, `<td class="verso-table__`); n < 7 {
		t.Errorf("every cell of a row stands at its top, got %d:\n%s", n, got)
	}
	declares(t, "table", ".verso-table :where(td, th)", "vertical-align: top")
}

// TestInterfaceRowOpensItsDrawerInPlace: a row whose info act's address is
// its panel hosts the panel scope and ships the empty frame, and the info act
// opens it in place; any other act, and a row without a panel, is a plain
// link. The name is words, not a control.
func TestInterfaceRowOpensItsDrawerInPlace(t *testing.T) {
	details := "/plugins/interfaces/details?device=br-lan"
	acts := func(details string) []TableRowAct {
		return []TableRowAct{{Icon: "info", Title: "Details", Href: details}, {Icon: "trash-2", Title: "Delete", Href: "/plugins/interfaces/delete?network=lan&device="}}
	}
	table := &Table{Style: "interfaces", Columns: inventoryColumns(), Rows: []TableRow{
		{ID: "br-lan", Panel: details, Cells: []TableCell{{Text: "br-lan"}, {}, {}, {}, {Actions: acts(details)}}},
		{ID: "eth2", Cells: []TableCell{{Text: "eth2"}, {}, {}, {}, {Actions: acts("/plugins/interfaces/details?device=eth2")}}},
	}}
	got := render(t, newRenderer(t), table)
	if strings.Count(got, `x-data="modal"`) != 1 || strings.Count(got, `data-verso-panel-url="`+details+`"`) != 1 {
		t.Errorf("one row hosts one panel frame:\n%s", got)
	}
	if !strings.Contains(got, `<a href="`+details+`" @click.prevent="showPanel" aria-label="Details br-lan"`) {
		t.Errorf("the info act opens the panel in place:\n%s", got)
	}
	if strings.Count(got, `@click.prevent="showPanel"`) != 1 {
		t.Errorf("only the panel's own address opens in place:\n%s", got)
	}
	if strings.Contains(got, `class="verso-table__identity"`) && strings.Contains(got, `<button type="button"`) {
		t.Errorf("an inventory row's name is words, not a control:\n%s", got)
	}
}

// An interface's type and address are machine cells, the value role's mono
// form, as every listing's are.
func TestInterfaceTypeAndAddressAreMachineCells(t *testing.T) {
	table := &Table{Style: "interfaces", Columns: inventoryColumns(), Rows: []TableRow{
		{ID: "br-lan", Cells: []TableCell{{Text: "br-lan"}, {Text: "bridge"}, {Text: "192.0.2.1/24"}, {Text: "Up", Variant: "success"}, {}}},
	}}
	got := render(t, newRenderer(t), table)
	for _, want := range []string{
		`<td data-verso-verbatim data-verso-machine class="verso-table__cell text-body"><span class="flex min-w-0 items-center"><span class="min-w-0 break-words">bridge</span>`,
		`<td data-verso-verbatim data-verso-machine class="verso-table__cell text-ink"><span class="flex min-w-0 items-center"><span class="min-w-0 break-words">192.0.2.1/24</span>`,
		`<td class="verso-table__cell"><span class="verso-table__state" data-verso-tone="success">`,
		`<td class="verso-table__acts">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	declares(t, "table", ".verso-table__cell[data-verso-machine]", "@apply type-value")
}

// On a phone the inventory keeps what says which interface and whether it is
// up: the tree, the name with its networks, the state, the acts. The type and
// the address are the interface's drawer's to tell; the state, whose column a
// phone has no room for, rides on the name's line. A name is never broken
// inside a word ("br-la|n").
func TestInterfaceInventoryOnAPhoneKeepsTheNameStateAndActs(t *testing.T) {
	table := &Table{Style: "interfaces", Columns: inventoryColumns(), Rows: []TableRow{
		{ID: "br-lan", Panel: "/plugins/interfaces/details?device=br-lan", Cells: []TableCell{{Text: "br-lan"}, {Text: "bridge"}, {Text: "192.0.2.1/24"}, {Text: "Up", Variant: "success"}, {}}},
	}}
	got := render(t, newRenderer(t), table)
	for _, want := range []string{
		`data-verso-tree`,
		`<colgroup><col><col><col><col><col></colgroup>`,
		`<span class="verso-table__title"><span class="break-words type-value text-ink" data-verso-verbatim>br-lan</span><span data-verso-inventory-state class="verso-table__state" data-verso-tone="success">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// The tree holds its columns' widths and is never narrower than 53.5rem
	// beside a phone; on a phone the type, the address and the state's own
	// column are left out, heads and widths with them, and the state stands
	// on the name's line only there.
	declares(t, "table", ".verso-table[data-verso-tree]", "min-width: 53.5rem")
	for col, width := range map[string]string{"2": "30", "3": "44", "4": "32", "5": "39"} {
		declares(t, "table", ".verso-table[data-verso-tree] > colgroup > col:nth-child("+col+")", "width: calc(var(--spacing) * "+width+")")
	}
	if rule := cssRule(blockCSS(t, "table"), ".verso-table[data-verso-tree] > colgroup > col:nth-child(6)"); rule != "" {
		t.Errorf("the tree has five columns, not six:\n%s", rule)
	}
	declares(t, "table", ".verso-table[data-verso-tree] > :is(thead, tbody) > tr > :nth-child(n+2):nth-child(-n+4),\n  .verso-table[data-verso-tree] > colgroup > col:nth-child(n+2):nth-child(-n+4)", "display: none")
	declares(t, "table", ".verso-table__state:is(.verso-table__title *)", "display: none")
	// The state is its packet and its word on one line, a link that is up
	// written in Ink.
	declares(t, "table", ".verso-table__state", "display: flex", "gap: calc(var(--spacing) * 2)")
	declares(t, "table", `.verso-table__state[data-verso-tone="success"]`, "color: var(--color-ink)")
	if strings.Contains(got, "break-all") {
		t.Errorf("an interface's name should break only where its words part:\n%s", got)
	}
}

// TestNoRowUnfolds: no listing unfolds its rows. A plugin built against an
// older SDK may still send `expanded`; the row renders as itself, with no
// unfold control and nothing under it, in the inventory and in any table. The
// stylesheets, the scripts and the grid keep nothing that only an unfolding
// row used.
func TestNoRowUnfolds(t *testing.T) {
	for _, style := range []string{"interfaces", ""} {
		w, err := Decode([]byte(`{"type":"table","style":"` + style + `","columns":[{"label":"Device","kind":"reference"},{"label":"State","kind":"status"},{"kind":"actions"}],
			"rows":[{"id":"br-lan","cells":[{"text":"br-lan"},{"text":"Up"},{}],"expanded":[{"type":"text","markdown":"what unfolded"}]}]}`))
		if err != nil {
			t.Fatalf("%q: decode: %v", style, err)
		}
		got := render(t, newRenderer(t), w)
		for _, absent := range []string{"data-verso-expand-row", "data-verso-expanded-row", "verso-table__sheet", "verso-table__unfold", "verso-table__toggle", "what unfolded"} {
			if strings.Contains(got, absent) {
				t.Errorf("%q: a row carries %q:\n%s", style, absent, got)
			}
		}
	}
	for block, selectors := range map[string][]string{
		"table":   {".verso-table__expand", ".verso-table__identity:focus-visible", ".verso-table__toggle", ".verso-table__sheet", ".verso-table__unfold", ".verso-table__hinge", ".verso-table__parts", `.verso-table__row[data-verso-expanded="true"]`},
		"arrival": {`.verso-arrival[data-verso-arrival="rise"]:where([data-verso-unfolding] *)`, `.verso-arrival[data-verso-arrival="draw"]:where([data-verso-unfolding] *)`},
		"grid":    {`.verso-grid__track:where(.verso-grid[data-verso-style="facts"] > *)`},
		"code":    {`.verso-code:is(.verso-grid[data-verso-style="facts"] .verso-grid__track > *)`},
	} {
		css := blockCSS(t, block)
		for _, sel := range selectors {
			if cssRule(css, sel) != "" {
				t.Errorf("blocks/%s.css keeps %s", block, sel)
			}
		}
		for _, word := range []string{"unfold", "expanded row", "data-verso-expanded"} {
			if strings.Contains(css, word) {
				t.Errorf("blocks/%s.css still speaks of %q", block, word)
			}
		}
	}
	for _, script := range []string{"verso-tables.js", "verso-page.js", "verso.js"} {
		js, err := os.ReadFile(filepath.Join("..", "server", "assets", script))
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"versoExpandRow", "data-verso-expand", "versoStagger", "versoRollFacts", "versoFactValues", "versoRoll", "expanded row", "unfold"} {
			if strings.Contains(string(js), word) {
				t.Errorf("%s still holds %q", script, word)
			}
		}
	}
	grid := render(t, newRenderer(t), &Grid{Style: "facts", Columns: 2, Children: []Widget{&Text{Markdown: "a"}, &Text{Markdown: "b"}}})
	if strings.Contains(grid, `data-verso-style="facts"`) || strings.Contains(grid, "data-verso-facts") {
		t.Errorf("no grid lays facts out in two columns for an unfolded row:\n%s", grid)
	}
}

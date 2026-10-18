// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package widget

import (
	"strings"
	"testing"
)

func TestInterfaceTreeKeepsBranchesThroughExpandedDescendants(t *testing.T) {
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
	declares(t, "table", ".verso-table[data-verso-tree] .verso-table__head:first-of-type", "padding-inline-start: calc(var(--spacing) * 29.25)")
	// Every cell of a row stands at its top: each is the table block's, and
	// the block stands every cell of its table at the row's top.
	if !strings.Contains(got, `<table class="verso-table"`) {
		t.Errorf("the inventory is not the table block:\n%s", got)
	}
	if n := strings.Count(got, `<td class="verso-table__`); n < 7 {
		t.Errorf("every cell of a row stands at its top, got %d:\n%s", n, got)
	}
	declares(t, "table", ".verso-table :where(td, th)", "vertical-align: top")
}

// TestInterfaceRowOpensItsEditorInPlace: a row whose pencil's address is its
// panel hosts the panel scope and ships the empty frame, and the pencil opens
// it in place; any other act, and a row without a panel, is a plain link.
func TestInterfaceRowOpensItsEditorInPlace(t *testing.T) {
	edit := "/plugins/interfaces/edit?network=lan&device="
	acts := func(edit string) []TableRowAct {
		return []TableRowAct{{Icon: "trash-2", Title: "Delete", Href: "/plugins/interfaces/delete?network=lan&device="}, {Icon: "square-pen", Title: "Edit", Href: edit}}
	}
	table := &Table{Style: "interfaces", Columns: []TableColumn{{Kind: "reference"}, {Kind: "keyword"}, {Kind: "mono"}, {Kind: "mono"}, {Kind: "status"}, {Kind: "actions"}}, Rows: []TableRow{
		{ID: "br-lan", Panel: edit, Cells: []TableCell{{Text: "br-lan"}, {}, {}, {}, {}, {Actions: acts(edit)}}},
		{ID: "eth2", Cells: []TableCell{{Text: "eth2"}, {}, {}, {}, {}, {Actions: acts("/plugins/interfaces/edit?network=&device=eth2")}}},
	}}
	r := newRenderer(t)
	got := render(t, r, table)
	escaped := strings.ReplaceAll(edit, "&", "&amp;")
	if strings.Count(got, `x-data="modal"`) != 1 || strings.Count(got, `data-verso-panel-url="`+escaped+`"`) != 1 {
		t.Errorf("one row hosts one panel frame:\n%s", got)
	}
	if !strings.Contains(got, `<a href="`+escaped+`" @click.prevent="showPanel" aria-label="Edit br-lan"`) {
		t.Errorf("the pencil opens the panel in place:\n%s", got)
	}
	if strings.Count(got, `@click.prevent="showPanel"`) != 1 {
		t.Errorf("only the panel's own address opens in place:\n%s", got)
	}
}

// On a phone the inventory keeps what says which interface and whether it is
// up: the tree, the name with its networks, the state, the acts. The type,
// the address and the MAC are the interface's panel's to tell; the state,
// whose column a phone has no room for, rides on the name's line. A name is
// never broken inside a word ("br-la|n").
func TestInterfaceInventoryOnAPhoneKeepsTheNameStateAndActs(t *testing.T) {
	table := &Table{Style: "interfaces", Columns: []TableColumn{{Label: "Device · network"}, {Label: "Type"}, {Label: "Address"}, {Label: "MAC address"}, {Label: "State"}, {Kind: "actions"}}, Rows: []TableRow{
		{ID: "br-lan", Panel: "/plugins/interfaces/br-lan/edit", Cells: []TableCell{{Text: "br-lan"}, {Text: "bridge"}, {Text: "192.0.2.1/24"}, {Text: "aa:bb:cc:dd:ee:ff"}, {Text: "Up", Variant: "success"}, {}}},
	}}
	got := render(t, newRenderer(t), table)
	for _, want := range []string{
		`data-verso-tree`,
		`<colgroup><col><col><col><col><col><col></colgroup>`,
		`<span class="verso-table__title"><span class="break-words type-value text-ink" data-verso-verbatim>br-lan</span><span data-verso-inventory-state class="verso-table__state" data-verso-tone="success">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// The tree holds its columns' widths and is never narrower than 66rem
	// beside a phone; on a phone the type, the address, the MAC and the
	// state's own column are left out, heads and widths with them, and the
	// state stands on the name's line only there.
	declares(t, "table", ".verso-table[data-verso-tree]", "min-width: 66rem")
	for col, width := range map[string]string{"2": "30", "3": "44", "4": "50", "5": "32", "6": "39"} {
		declares(t, "table", ".verso-table[data-verso-tree] > colgroup > col:nth-child("+col+")", "width: calc(var(--spacing) * "+width+")")
	}
	declares(t, "table", ".verso-table[data-verso-tree] > :is(thead, tbody) > tr > :nth-child(n+2):nth-child(-n+5),\n  .verso-table[data-verso-tree] > colgroup > col:nth-child(n+2):nth-child(-n+5)", "display: none")
	declares(t, "table", ".verso-table__state:is(.verso-table__title *)", "display: none")
	// The state is its packet and its word on one line, a link that is up
	// written in Ink.
	declares(t, "table", ".verso-table__state", "display: flex", "gap: calc(var(--spacing) * 2)")
	declares(t, "table", `.verso-table__state[data-verso-tone="success"]`, "color: var(--color-ink)")
	if strings.Contains(got, "break-all") {
		t.Errorf("an interface's name should break only where its words part:\n%s", got)
	}
}

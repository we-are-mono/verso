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
	for _, want := range []string{
		`class="pointer-events-none absolute top-0 left-4 h-10`,
		`class="pointer-events-none absolute top-10 bottom-0 left-4`,
		`preserveAspectRatio="none"`, `vector-effect="non-scaling-stroke"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "align-top"); n < 6 {
		t.Errorf("every cell of a row stands at its top, got %d:\n%s", n, got)
	}
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

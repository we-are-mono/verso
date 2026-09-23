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
	// The physical marker is centered on its own stem, at x=8.
	if rows[0].TreeNodeX != 4 || rows[0].TreeNodeCenter != 8 || !strings.Contains(rows[0].TreePath, "M8 22V44") {
		t.Fatalf("root marker and branch disagree: %+v", rows[0])
	}
	for _, want := range []struct {
		index              int
		path, continuation string
	}{
		{1, "M8 0V44", "M8 0V1M32 0V1"}, // Both the ancestor and child continue below this row.
		{2, "M32 0V22", "M8 0V1"},       // PPP ends, but the next VLAN still needs the outer stem.
		{3, "M8 0V22", ""},              // Last VLAN ends the physical port's branch.
		{4, "M8 22V44", "M8 0V1"},       // The next root starts a separate bridge branch.
		{5, "M8 0V22", ""},              // Only lan0 belongs to that bridge.
		{6, "M8 22H66", ""},             // WAN remains independent.
	} {
		got := rows[want.index]
		if !strings.Contains(got.TreePath, want.path) || got.TreeContinuation != want.continuation {
			t.Errorf("%s: path=%q continuation=%q", got.ID, got.TreePath, got.TreeContinuation)
		}
	}
	got := render(t, r, table)
	// A wrapped row hangs from its first line: the node and its branch stay on
	// the first 44px, and only the trunks that run on to later rows stretch
	// down the rest of the row, without changing their stroke.
	for _, want := range []string{
		`class="pointer-events-none absolute top-0 left-4 h-11`,
		`class="pointer-events-none absolute top-11 bottom-0 left-4`,
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

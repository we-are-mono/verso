// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheRetiredInterfacesStyleIsTheFlatTable: the interface inventory once
// had a table shape of its own. A plugin still asking for it gets the one
// flat table every listing is, and nothing of the inventory's shape is left
// in the templates, the stylesheet or the scripts.
func TestTheRetiredInterfacesStyleIsTheFlatTable(t *testing.T) {
	w, err := Decode([]byte(`{"type":"table","style":"interfaces","columns":[{"label":"Device","kind":"reference"},{"label":"State","kind":"status"},{"kind":"actions"}],
		"rows":[{"id":"br-lan","cells":[{"text":"br-lan"},{"text":"up","variant":"success"},{}]},{"id":"lan0","depth":1,"cells":[{"text":"lan0"},{"text":"up"},{}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := render(t, newRenderer(t), w)
	if !strings.Contains(got, `data-verso-listing`) || !strings.Contains(got, `data-verso-row-id="lan0"`) {
		t.Errorf("the rows stand in the flat table:\n%s", got)
	}
	for _, absent := range []string{"data-verso-tree", "data-verso-inventory", "verso-table__identity", "verso-table__state", "data-verso-depth"} {
		if strings.Contains(got, absent) {
			t.Errorf("the inventory's own shape is gone, found %q:\n%s", absent, got)
		}
	}
	if _, err := os.Stat(filepath.Join("templates", "table-interfaces.html.tmpl")); !os.IsNotExist(err) {
		t.Error("templates/table-interfaces.html.tmpl is still shipped")
	}
	css := blockCSS(t, "table")
	for _, word := range []string{"data-verso-tree", "verso-table__identity", "verso-table__state", "verso-table__actions", "data-verso-depth", "data-verso-filtered"} {
		if strings.Contains(css, word) {
			t.Errorf("blocks/table.css keeps the inventory's %s", word)
		}
	}
	js, err := os.ReadFile(filepath.Join("..", "server", "assets", "verso-tables.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"versoFiltered", "data-verso-inventory]"} {
		if strings.Contains(string(js), word) {
			t.Errorf("verso-tables.js still holds %q", word)
		}
	}
}

// TestAnActSlotWithNoActHoldsItsPlace: an act with no glyph is a slot the
// row has no act for. It holds its place, so each act stands in its own
// column down the rows, and draws nothing.
func TestAnActSlotWithNoActHoldsItsPlace(t *testing.T) {
	got := render(t, newRenderer(t), &Table{Columns: []TableColumn{{Label: "Device", Kind: "mono"}, {Kind: "actions"}}, Rows: []TableRow{
		{ID: "eth0", Cells: []TableCell{{Text: "eth0"}, {Actions: []TableRowAct{{Icon: "square-pen", Title: "Edit", Href: "/edit"}, {}}}}},
	}})
	if !strings.Contains(got, `<span class="size-7 shrink-0" aria-hidden="true"></span>`) {
		t.Errorf("the empty slot holds its place:\n%s", got)
	}
	if strings.Count(got, `aria-disabled="true"`) != 0 {
		t.Errorf("the empty slot draws no inert act:\n%s", got)
	}
}

// TestNoRowUnfolds: no listing unfolds its rows. A plugin built against an
// older SDK may still send `expanded`; the row renders as itself, with no
// unfold control and nothing under it. The stylesheets, the scripts and the
// grid keep nothing that only an unfolding row used.
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

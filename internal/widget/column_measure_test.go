// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestColumnMeasuresAreAClosedSet: a column's width is one of a few measures
// named by what the column holds, never a CSS length a plugin makes up — so
// every listing's address column is the same width, and two listings line up.
func TestColumnMeasuresAreAClosedSet(t *testing.T) {
	want := map[ColumnMeasure]string{
		MeasureMark:    "2rem",
		MeasureCount:   "4.5rem",
		MeasureShort:   "6rem",
		MeasureWord:    "9rem",
		MeasureAddress: "12.5rem",
		MeasureName:    "14rem",
		MeasureLong:    "17rem",
	}
	for m, css := range want {
		if got := m.CSS(); got != css {
			t.Errorf("measure %q = %q, want %q", m, got, css)
		}
	}
	if got := MeasureGrow.CSS(); got != "" {
		t.Errorf("a column with no measure grows; got width %q", got)
	}
}

// TestColumnMeasureRendersAsItsWidth: the table carries each measure into its
// colgroup, and a column that states none shares what is left.
func TestColumnMeasureRendersAsItsWidth(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{
			{Label: "Address", Kind: "mono", Width: MeasureAddress},
			{Label: "Comment"},
			{Label: "Hits", Kind: "num", Width: MeasureCount},
		},
		Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "192.168.1.1"}, {Text: "gateway"}, {Text: "3"}}}},
	})
	for _, want := range []string{`<col style="width:12.5rem">`, `<col>`, `<col style="width:4.5rem">`} {
		if !strings.Contains(got, want) {
			t.Errorf("colgroup missing %q:\n%s", want, got)
		}
	}
}

// TestUnknownColumnMeasureIsRefused: a width outside the set — a raw CSS length
// above all — fails the decode loudly, like an unknown widget type does, rather
// than drawing a column no other listing has.
func TestUnknownColumnMeasureIsRefused(t *testing.T) {
	for _, width := range []string{`"6rem"`, `"wide"`, `"120px"`} {
		_, err := Decode([]byte(`{"type":"table","columns":[{"label":"Port","width":` + width + `}],"rows":[]}`))
		if err == nil {
			t.Errorf("width %s decoded; want it refused", width)
			continue
		}
		if !strings.Contains(err.Error(), "address") {
			t.Errorf("width %s: the error should name the measures there are: %v", width, err)
		}
	}
	if _, err := Decode([]byte(`{"type":"table","columns":[{"label":"Port","width":"short"},{"label":"Rule"}],"rows":[]}`)); err != nil {
		t.Errorf("a named measure and an unmeasured column decode: %v", err)
	}
}

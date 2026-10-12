// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// renderIn renders w for a reader whose language translates through t.
func renderIn(t *testing.T, w Widget, tr func(string) string) string {
	t.Helper()
	var b strings.Builder
	if err := newRenderer(t).RenderWithToken(&b, w, "", "sl", tr); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	return normalizeHTML(b.String())
}

func decimalComma(s string) string {
	if s == "." {
		return ","
	}
	return s
}

// TestAPluginRateIsWrittenByTheShell: a plugin hands over a rate as bits per
// second and the shell writes it as every rate is written, in the reader's
// language, set in sans whatever the plugin asked of the value's type.
func TestAPluginRateIsWrittenByTheShell(t *testing.T) {
	link := 2.5e9
	props := func() Widget {
		return &Properties{Items: []Property{{Label: "Link", Mono: true, Rate: &link}}}
	}
	if got := render(t, newRenderer(t), props()); !strings.Contains(got, ">2.5 Gb/s<") || strings.Contains(got, "font-mono") {
		t.Errorf("a property's rate = %s", got)
	}
	if got := renderIn(t, props(), decimalComma); !strings.Contains(got, ">2,5 Gb/s<") {
		t.Errorf("a property's rate in a decimal-comma language = %s", got)
	}

	table := func() Widget {
		return &Table{
			Columns: []TableColumn{{Label: "Device", Kind: "mono"}, {Label: "Type", Kind: "keyword"}},
			Rows: []TableRow{
				{ID: "eth0", Cells: []TableCell{{Text: "eth0"}, {Text: "port", Rate: &link}}},
				{ID: "eth1", Cells: []TableCell{{Text: "eth1"}, {Text: "port"}}},
			},
		}
	}
	got := render(t, newRenderer(t), table())
	if !strings.Contains(got, ">port · 2.5 Gb/s<") || !strings.Contains(got, ">port<") {
		t.Errorf("a cell's rate rides after its text: %s", got)
	}
	if got := renderIn(t, table(), decimalComma); !strings.Contains(got, ">port · 2,5 Gb/s<") {
		t.Errorf("a cell's rate in a decimal-comma language = %s", got)
	}
}

// TestADecodedRateIsBitsPerSecond: the vocabulary carries the rate as a
// number, "rate" on a property and on a cell.
func TestADecodedRateIsBitsPerSecond(t *testing.T) {
	w, err := Decode([]byte(`{"type":"properties","items":[{"label":"Link","value":"","rate":1000000000}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := render(t, newRenderer(t), w); !strings.Contains(got, ">1 Gb/s<") {
		t.Errorf("a decoded rate = %s", got)
	}
}

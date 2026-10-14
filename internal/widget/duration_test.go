// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// writtenSpan is 52 d 8 h as the shell draws it: each figure at 500 in its
// context's ink, each unit receding to Meta at 400. The space between two
// parts is there; render() folds whitespace between tags away.
var writtenSpan = written("52", "d", "8", "h")

// written is how render() shows a span of the given figures and units, in
// pairs: written("2", "h", "14", "min").
func written(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		b.WriteString(`<span class="verso-duration-figure">` + pairs[i] + `</span>&nbsp;<span class="verso-duration-unit">` + pairs[i+1] + `</span>`)
	}
	return b.String()
}

// secs is a duration the Overview holds, in seconds.
func secs(n int64) *int64 { return &n }

// TestAPluginDurationIsWrittenByTheShell: a plugin hands over a stretch of
// time in seconds and the shell writes it as every duration is written, its
// figures apart from their units, in sans whatever the plugin asked of the
// value's type.
func TestAPluginDurationIsWrittenByTheShell(t *testing.T) {
	up := int64(4523580)
	got := render(t, newRenderer(t), &Properties{Items: []Property{{Label: "Uptime", Mono: true, Key: "uptime", Duration: &up}}})
	if !strings.Contains(got, writtenSpan) || strings.Contains(got, "font-mono") {
		t.Errorf("a property's duration = %s", got)
	}
	if !strings.Contains(got, `data-verso-prop="uptime"`) {
		t.Errorf("a live duration keeps its key: %s", got)
	}

	lease := int64(4523580)
	table := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Address", Kind: "mono"}, {Label: "Expires in", Kind: "runtime"}},
		Rows: []TableRow{
			{ID: "a", Cells: []TableCell{{Text: "10.0.0.2"}, {Duration: &lease}}},
			{ID: "b", Cells: []TableCell{{Text: "10.0.0.3"}, {Text: "never"}}},
		},
	})
	if !strings.Contains(table, writtenSpan) || !strings.Contains(table, ">never<") {
		t.Errorf("a cell's duration = %s", table)
	}
}

// TestADecodedDurationIsSeconds: the vocabulary carries the duration as a
// number, "duration" on a property and on a cell.
func TestADecodedDurationIsSeconds(t *testing.T) {
	w, err := Decode([]byte(`{"type":"properties","items":[{"label":"Uptime","value":"","duration":8040}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := render(t, newRenderer(t), w); !strings.Contains(got, written("2", "h", "14", "min")) {
		t.Errorf("a decoded duration = %s", got)
	}
}

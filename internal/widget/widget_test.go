// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "testing"

func TestDecodeTable(t *testing.T) {
	in := []byte(`{"type":"table","columns":["A","B"],"rows":[["1","2"],["3","4"]]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}

	tbl, ok := w.(*Table)
	if !ok {
		t.Fatalf("Decode: got %T, want *Table", w)
	}
	if got, want := tbl.Columns, []string{"A", "B"}; !equalStrings(got, want) {
		t.Errorf("Columns = %v, want %v", got, want)
	}
	if len(tbl.Rows) != 2 || tbl.Rows[1][0] != "3" || tbl.Rows[0][1] != "2" {
		t.Errorf("Rows = %v, want [[1 2] [3 4]]", tbl.Rows)
	}
}

func TestDecodeUnknownType(t *testing.T) {
	if _, err := Decode([]byte(`{"type":"nope"}`)); err == nil {
		t.Fatal("Decode: want error for unknown type, got nil")
	}
}

func TestDecodeMissingType(t *testing.T) {
	if _, err := Decode([]byte(`{"columns":[]}`)); err == nil {
		t.Fatal("Decode: want error for missing type, got nil")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

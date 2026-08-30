// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRenderGrid(t *testing.T) {
	r := newRenderer(t)
	g := &Grid{Columns: 2, Children: []Widget{
		&Stat{Label: "A", Value: "1"},
		&Stat{Label: "B", Value: "2"},
	}}
	got := render(t, r, g)
	if !strings.Contains(got, "grid-cols-2") {
		t.Errorf("two-column grid classes missing: %s", got)
	}
	for _, want := range []string{"A", "B"} {
		if !strings.Contains(got, want) {
			t.Errorf("grid child %q not rendered: %s", want, got)
		}
	}
}

// TestGridColumnsResponsive checks the count→responsive-class mapping the shell owns.
func TestGridColumnsResponsive(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{1: "grid-cols-1", 2: "grid-cols-2", 3: "md:grid-cols-3", 4: "md:grid-cols-4"}
	for cols, want := range cases {
		got := render(t, r, &Grid{Columns: cols, Children: []Widget{&Stat{Label: "x", Value: "1"}}})
		if !strings.Contains(got, want) {
			t.Errorf("columns=%d: want class %q in:\n%s", cols, want, got)
		}
	}
}

func TestRenderFormGrid(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 3, Children: []Widget{
		&Field{Name: "password", Label: "New password", Kind: "password"},
		&Field{Name: "confirm", Label: "Repeat password", Kind: "password"},
	}})
	for _, want := range []string{"grid-cols-1 md:grid-cols-3", "gap-6", "New password", "Repeat password"} {
		if !strings.Contains(got, want) {
			t.Errorf("form grid missing %q:\n%s", want, got)
		}
	}
}

// TestDecodeGridChildren covers UnmarshalJSON: a grid decodes its children through
// the shared Decode, so an unknown child fails loudly rather than vanishing.
func TestDecodeGridChildren(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"grid","columns":3,"children":[
		{"type":"stat","label":"Devices","value":"9"},
		{"type":"badge","variant":"success","text":"up"}
	]}`))
	if err != nil {
		t.Fatalf("decode grid: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"md:grid-cols-3", "Devices", "up"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded grid missing %q:\n%s", want, got)
		}
	}

	if _, err := Decode([]byte(`{"type":"grid","children":[{"type":"nope"}]}`)); err == nil {
		t.Error("grid with an unknown child should fail to decode, not vanish")
	}
}

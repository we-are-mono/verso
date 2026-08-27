// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderCanvas: the canvas is a light rounded panel (slate-50 fill, slate-200
// hairline) that renders its children inside, with uniform default padding (3rem
// on every side).
func TestRenderCanvas(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Canvas{Children: []Widget{&Text{Markdown: "inside the canvas"}}})
	for _, want := range []string{
		"rounded-2xl", "border-slate-200", "bg-slate-50",
		"padding: 3rem",     // uniform default padding
		"inside the canvas", // the child rendered within
	} {
		if !strings.Contains(got, want) {
			t.Errorf("canvas missing %q:\n%s", want, got)
		}
	}
}

// TestRenderCanvasPad: the uniform padding is configurable per canvas (Tailwind
// spacing units → rem).
func TestRenderCanvasPad(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Canvas{Pad: 8, Children: []Widget{&Text{Markdown: "x"}}})
	if !strings.Contains(got, "padding: 2rem") { // 8 * 0.25rem
		t.Errorf("Pad 8 should render 2rem padding:\n%s", got)
	}
}

// TestDecodeCanvas: children decode recursively, and an unknown child fails loudly.
func TestDecodeCanvas(t *testing.T) {
	w, err := Decode([]byte(`{"type":"canvas","children":[{"type":"text","markdown":"hi"}]}`))
	if err != nil {
		t.Fatalf("decode canvas: %v", err)
	}
	c, ok := w.(*Canvas)
	if !ok {
		t.Fatalf("decoded %T, want *Canvas", w)
	}
	if len(c.Children) != 1 {
		t.Errorf("canvas children not decoded: %+v", c.Children)
	}
	if _, err := Decode([]byte(`{"type":"canvas","children":[{"type":"nope"}]}`)); err == nil {
		t.Error("an unknown canvas child should fail loudly")
	}
}

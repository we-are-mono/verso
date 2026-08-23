// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func newRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func render(t *testing.T, r *Renderer, w Widget) string {
	t.Helper()
	var b strings.Builder
	if err := r.Render(&b, w); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return strings.TrimSpace(b.String())
}

func TestRenderTable(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []string{"Name", "Status"},
		Rows:    [][]string{{"wan", "up"}, {"lan", "down"}},
	}

	got := render(t, r, tbl)
	want := `<table class="w-full border-collapse overflow-hidden rounded-lg border border-verso-border"><thead><tr><th scope="col" class="border-b border-verso-border px-3.5 py-2.5 text-left text-verso-muted font-normal text-sm tracking-wider">Name</th><th scope="col" class="border-b border-verso-border px-3.5 py-2.5 text-left text-verso-muted font-normal text-sm tracking-wider">Status</th></tr></thead><tbody class="divide-y divide-verso-border"><tr><td class="px-3.5 py-2.5 text-left">wan</td><td class="px-3.5 py-2.5 text-left">up</td></tr><tr><td class="px-3.5 py-2.5 text-left">lan</td><td class="px-3.5 py-2.5 text-left">down</td></tr></tbody></table>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderTableEscapesCells(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []string{"x"},
		Rows:    [][]string{{`<script>alert(1)</script>`}},
	}

	got := render(t, r, tbl)
	if strings.Contains(got, "<script>") {
		t.Errorf("cell content not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("escaped cell content missing: %s", got)
	}
}

func TestRenderUnknownWidgetErrors(t *testing.T) {
	r := newRenderer(t)
	var b strings.Builder
	if err := r.Render(&b, fakeWidget{}); err == nil {
		t.Fatal("Render: want error for unknown widget type, got nil")
	}
}

type fakeWidget struct{}

func (fakeWidget) isWidget() {}

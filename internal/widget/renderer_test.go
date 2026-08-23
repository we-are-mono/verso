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

func TestRenderCardChrome(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{Title: "empty"})
	want := `<section class="rounded-lg border border-verso-border"><header class="border-b border-verso-border px-4 py-3"><h3 class="text-sm font-medium text-verso-fg">empty</h3></header><div class="px-4 py-4 space-y-4"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderCardWithoutTitleOmitsHeader(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{})
	want := `<section class="rounded-lg border border-verso-border"><div class="px-4 py-4 space-y-4"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestRenderCardNestsChild proves a container renders its children through the
// same renderer, inside its own body — the composition the schema bet depends on.
func TestRenderCardNestsChild(t *testing.T) {
	r := newRenderer(t)
	card := &Card{
		Title:    "Status",
		Children: []Widget{&Table{Columns: []string{"A"}, Rows: [][]string{{"1"}}}},
	}

	got := render(t, r, card)
	body := strings.Index(got, `<div class="px-4 py-4 space-y-4">`)
	table := strings.Index(got, "<table")
	if body < 0 || table < 0 || table < body {
		t.Errorf("nested table not rendered inside card body: %s", got)
	}
}

func TestRenderCardNestingDepth(t *testing.T) {
	r := newRenderer(t)
	card := &Card{Title: "outer", Children: []Widget{
		&Card{Title: "inner", Children: []Widget{
			&Table{Columns: []string{"A"}, Rows: [][]string{{"1"}}},
		}},
	}}

	got := render(t, r, card)
	if strings.Count(got, "<section") != 2 {
		t.Errorf("want 2 nested card sections, got: %s", got)
	}
	if !strings.Contains(got, "<table") {
		t.Errorf("innermost table missing: %s", got)
	}
}

func TestRenderCardEscapesTitle(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{Title: `<script>alert(1)</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("card title not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("escaped title missing: %s", got)
	}
}

func TestRenderFieldText(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "hostname", Label: "Hostname", Value: "OpenWrt", Datatype: "hostname"})
	for _, want := range []string{`name="hostname"`, `value="OpenWrt"`, "Hostname", "<input", `data-datatype="hostname"`} {
		if !strings.Contains(got, want) {
			t.Errorf("field missing %q: %s", want, got)
		}
	}
}

func TestRenderFieldSelectMarksSelected(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{
		Name: "tz", Label: "Timezone", Kind: "select", Value: "CET",
		Options: []Option{{Value: "UTC", Label: "UTC"}, {Value: "CET", Label: "Central"}},
	})
	if !strings.Contains(got, "<select") {
		t.Errorf("no <select>: %s", got)
	}
	if !strings.Contains(got, `value="CET" selected`) {
		t.Errorf("selected option not marked: %s", got)
	}
	if strings.Contains(got, `value="UTC" selected`) {
		t.Errorf("wrong option marked selected: %s", got)
	}
}

func TestRenderFieldError(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "h", Label: "H", Value: "bad host", Error: "must be a valid hostname"})
	if !strings.Contains(got, "must be a valid hostname") {
		t.Errorf("inline error not shown: %s", got)
	}
	if !strings.Contains(got, "border-verso-danger") {
		t.Errorf("errored field should carry the danger token: %s", got)
	}
}

func TestRenderFieldEscapesValue(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "h", Label: "H", Value: `"><script>alert(1)</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("field value not escaped: %s", got)
	}
}

// TestRenderListItemsPlusBlank proves the repeating widget: N item inputs plus a
// trailing blank add-slot, all sharing the name so they post as a multi-value
// field.
func TestRenderListItemsPlusBlank(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &List{Name: "server", Label: "NTP servers", Datatype: "host",
		Items: []string{"0.pool.ntp.org", "1.pool.ntp.org"}})
	if n := strings.Count(got, `name="server"`); n != 3 {
		t.Errorf("want 3 inputs (2 items + 1 blank), got %d: %s", n, got)
	}
	for _, want := range []string{"NTP servers", "0.pool.ntp.org", "1.pool.ntp.org", "Add"} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing %q", want)
		}
	}
}

// TestRenderListPerItemError shows how a repeating widget reports which row
// failed: an error keyed by the item index.
func TestRenderListPerItemError(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &List{Name: "server", Label: "NTP", Items: []string{"ok.example.com", "bad host"},
		Errors: map[string]string{"1": "must be a hostname or IP address"}})
	if !strings.Contains(got, "must be a hostname or IP address") {
		t.Errorf("per-item error missing: %s", got)
	}
}

func TestRenderFormDefaultsSubmitLabel(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Form{Fields: []Widget{&Field{Name: "h", Label: "H"}}})
	if !strings.Contains(got, `<form method="post"`) {
		t.Errorf("no post form: %s", got)
	}
	if !strings.Contains(got, ">Save<") {
		t.Errorf("default submit label 'Save' missing: %s", got)
	}
	if !strings.Contains(got, `name="h"`) {
		t.Errorf("nested field not rendered inside form: %s", got)
	}
}

func TestRenderFormSuccess(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Form{Submit: "Apply", Success: "Saved."})
	if !strings.Contains(got, "Saved.") {
		t.Errorf("success message missing: %s", got)
	}
	if !strings.Contains(got, ">Apply<") {
		t.Errorf("custom submit label missing: %s", got)
	}
}

func TestRenderRawMarkdown(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Raw{Markdown: "**bold** and `code`"})
	for _, want := range []string{"<strong>bold</strong>", "<code>code</code>", "Raw"} {
		if !strings.Contains(got, want) {
			t.Errorf("raw output missing %q: %s", want, got)
		}
	}
}

// TestRenderRawSanitizes is the bridge's safety net: source HTML is neutralised
// and dangerous URL schemes are stripped, while a safe link survives.
func TestRenderRawSanitizes(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Raw{Markdown: "<script>alert(1)</script>\n\n[x](javascript:alert(2)) and [ok](https://example.com)"})
	if strings.Contains(got, "<script>") {
		t.Errorf("raw HTML not neutralised: %s", got)
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: scheme not stripped: %s", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("safe link was dropped: %s", got)
	}
}

// TestRawUsageInstrumented proves the demand-signal counter (ADR-005 §5).
func TestRawUsageInstrumented(t *testing.T) {
	r := newRenderer(t)
	if r.RawUsage() != 0 {
		t.Fatalf("initial RawUsage = %d, want 0", r.RawUsage())
	}
	_ = render(t, r, &Raw{Markdown: "a"})
	_ = render(t, r, &Raw{Markdown: "b"})
	if r.RawUsage() != 2 {
		t.Errorf("RawUsage = %d, want 2", r.RawUsage())
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

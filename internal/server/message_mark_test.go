// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/widget"
)

// TestMessagesMarkWithASquareNotAGlyph: an error, a notice, a warning or a
// confirmation says its tone with the state mark — the small square in the
// tone's hue — never with an alert glyph. No template draws one.
func TestMessagesMarkWithASquareNotAGlyph(t *testing.T) {
	var files []string
	for _, dir := range []string{"templates", "../widget/templates"} {
		found, err := filepath.Glob(filepath.Join(dir, "*.tmpl"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, found...)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, glyph := range []string{`icon "circle-alert"`, `icon "triangle-alert"`, `icon "circle-check"`, `icon "info"`} {
			if strings.Contains(string(body), glyph) {
				t.Errorf("%s marks a message with %s; a message wears the square", file, glyph)
			}
		}
	}
	r, err := widget.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	form := &widget.Form{Error: "Port is taken.", Submit: "Save", Fields: []widget.Widget{&widget.Field{Name: "x"}}}
	if err := r.RenderWithToken(&out, form, "", "en", func(s string) string { return s }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `<span aria-hidden="true" class="mt-1.75 size-1.5 shrink-0 rounded-[1px] bg-crimson"></span>`) {
		t.Errorf("a form's error is marked with the crimson square:\n%s", out.String())
	}
}

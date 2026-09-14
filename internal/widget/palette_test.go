// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stockRamp matches a Tailwind stock colour — a ramp name with a numbered step, as
// a utility class or as a theme variable. The app's own colours are named for what
// they are (ink, rule, denim, marigold) and carry no number, so a number after a
// hue name is always a colour no design chose.
var stockRamp = regexp.MustCompile(
	`(?:\b(?:bg|text|border|divide|ring|fill|stroke|outline|from|via|to|accent|caret|placeholder|shadow)-|var\(--color-)` +
		`(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|` +
		`indigo|violet|purple|fuchsia|pink|rose)-[0-9]+`)

// TestNothingWearsAStockTailwindRamp walks every surface the shell paints — its
// widget templates, the page chrome's templates, the stylesheet's named rules and
// the client's own built markup — and fails on a stock Tailwind colour.
//
// The palette is the design's (palette.css, and the Palette canvas beside it): each
// token says what it is for, and the contrast of every pair was chosen. A stock ramp
// is a colour nobody chose, and it does not merely look slightly wrong — it breaks
// the one rule the whole look rests on, which is that the same thing is the same
// colour everywhere. That is how a live status dot came to be repainted in a green
// the page never renders, and how a meter changed shade the moment it updated.
//
// A test naming a ramp is not covered here and does not need to be: the stylesheet
// is built from the declared sources only (input.css's source(none)), so a class no
// surface renders is not compiled, and an assertion that names one fails honestly
// rather than finding its own string in the inlined CSS.
func TestNothingWearsAStockTailwindRamp(t *testing.T) {
	roots := []string{
		filepath.Join("templates"),
		filepath.Join("..", "server", "templates"),
		filepath.Join("..", "server", "assets", "input.css"),
	}
	for _, root := range roots {
		walk(t, root, func(path string, body string) {
			for _, line := range strings.Split(body, "\n") {
				if hit := stockRamp.FindString(line); hit != "" {
					t.Errorf("%s wears the stock Tailwind colour %q:\n  %s",
						path, hit, strings.TrimSpace(line))
				}
			}
		})
	}

	// The shell's client builds markup of its own, in the same treatments; a ramp
	// there is a row that looks different from the rows the server rendered beside
	// it. verso-dev.js and verso-boot.js paint nothing.
	scripts, err := filepath.Glob(filepath.Join("..", "server", "assets", "verso*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range scripts {
		base := filepath.Base(script)
		if base == "verso-dev.js" || base == "verso-boot.js" {
			continue
		}
		body, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if hit := stockRamp.FindString(line); hit != "" {
				t.Errorf("%s wears the stock Tailwind colour %q:\n  %s",
					script, hit, strings.TrimSpace(line))
			}
		}
	}
}

// walk hands every file under root (or root itself, if it is one) to check.
func walk(t *testing.T, root string, check func(path, body string)) {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		t.Skipf("%s not readable: %v", root, err)
	}
	if !info.IsDir() {
		body, err := os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
		check(root, string(body))
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		check(path, string(body))
	}
}

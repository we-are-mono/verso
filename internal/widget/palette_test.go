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
	// it. verso-dev.js paints nothing.
	scripts, err := filepath.Glob(filepath.Join("..", "server", "assets", "verso*.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range scripts {
		base := filepath.Base(script)
		if base == "verso-dev.js" {
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

// colourLiterals are the ways a colour can be written as a value instead of
// named as a palette token (palette spec). Each is a colour palette.css cannot
// reach: a palette change leaves it behind.
var colourLiterals = []*regexp.Regexp{
	// Tailwind's own white and black: a surface is Canvas, a label on a fill
	// On-fill. Neither is a colour of the palette.
	regexp.MustCompile(`\b(?:text|bg|border|ring|fill|stroke|outline|from|via|to|divide|placeholder|caret|accent|decoration|shadow)-(?:white|black)\b|--color-(?:white|black)\b`),
	// A colour function with a value in it, as a shadow in Ink once was.
	// No word boundary before it: in an arbitrary utility it follows an
	// underscore, shadow-[0_1px_2px_rgba(…)].
	regexp.MustCompile(`(?:rgba?|hsla?)\(\s*[0-9.]`),
}

// hexColour is a hex colour of 3, 4, 6 or 8 digits, alone or in an arbitrary
// class (bg-[#f9f7f3]). What stands before it decides whether it is a colour
// at all: see notAColour.
var hexColour = regexp.MustCompile(`#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})\b`)

// notAColour is what a # after these means instead: a character reference
// (&#160;), a link to a fragment (href="#add") or a reference to an element
// (url(#fill)).
var notAColour = []string{"&", `href="`, "url("}

// colourExceptions are literals kept on purpose, keyed by the file's name and
// the literal, each with the reason it is not a colour the palette owes.
var colourExceptions = map[string]string{
	"scroll-edge.css #000":    "the scroll edge's mask: only its alpha is read, it paints nothing",
	"icons.html.tmpl #1b1917": "Safari's pinned-tab colour: the brand's mark, not the page",
}

// literalColours is every colour literal in line of the file named file that is
// not kept on purpose.
func literalColours(file, line string) []string {
	var found []string
	for _, re := range colourLiterals {
		found = append(found, re.FindAllString(line, -1)...)
	}
	for _, at := range hexColour.FindAllStringIndex(line, -1) {
		before := line[:at[0]]
		reference := false
		for _, prefix := range notAColour {
			reference = reference || strings.HasSuffix(before, prefix)
		}
		if !reference {
			found = append(found, line[at[0]:at[1]])
		}
	}
	var hits []string
	for _, hit := range found {
		if _, kept := colourExceptions[file+" "+hit]; !kept {
			hits = append(hits, hit)
		}
	}
	return hits
}

// paintedSources is every file the shell paints from outside palette.css:
// both template sets, the stylesheets, the client's scripts that build markup,
// and the Go that renders.
func paintedSources(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{
		filepath.Join("templates", "*.tmpl"),
		filepath.Join("..", "server", "templates", "*.tmpl"),
		filepath.Join("..", "server", "assets", "input.css"),
		filepath.Join("..", "server", "assets", "blocks", "*.css"),
		filepath.Join("..", "server", "assets", "verso*.js"),
		"*.go",
		filepath.Join("..", "server", "*.go"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			base := filepath.Base(m)
			if strings.HasSuffix(base, "_test.go") || base == "verso-dev.js" {
				continue
			}
			files = append(files, m)
		}
	}
	if len(files) < 60 {
		t.Fatalf("found %d painted sources; the globs no longer reach them", len(files))
	}
	return files
}

// TestNothingPaintsALiteralColour walks every source the shell paints from and
// fails on a colour written as a value. palette.css is the one place a colour
// is written; everywhere else names its token, so the palette decides the
// whole look and nothing keeps an old colour behind.
func TestNothingPaintsALiteralColour(t *testing.T) {
	for _, path := range paintedSources(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(body), "\n") {
			for _, hit := range literalColours(filepath.Base(path), line) {
				t.Errorf("%s:%d writes the colour %q; name a palette token:\n  %s",
					path, n+1, hit, strings.TrimSpace(line))
			}
		}
	}
}

// fusedTokenClass is a palette class run into the class after it
// (text-on-filltransition-colors): neither compiles, so the label falls back
// to the inherited ink and the control loses its fill.
var fusedTokenClass = regexp.MustCompile(`\b(?:text-on-fill|bg-canvas|shadow-\(--shadow-[a-z-]+\))[A-Za-z0-9_\[(]`)

// TestTokenClassesStandApart fails where a palette class touches the next
// class with no space between them.
func TestTokenClassesStandApart(t *testing.T) {
	for _, path := range paintedSources(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(body), "\n") {
			if hit := fusedTokenClass.FindString(line); hit != "" {
				t.Errorf("%s:%d runs a palette class into the next: %q", path, n+1, hit)
			}
		}
	}
}

// TestTheLiteralGuardSeesWhatItShould pins what the guard catches and what it
// leaves, so a loosened pattern fails here rather than letting a literal by.
func TestTheLiteralGuardSeesWhatItShould(t *testing.T) {
	for _, line := range []string{
		`<span class="relative text-white opacity-0">`,
		`class="group-data-arranging/overview:text-white"`,
		`<div class="hover:bg-white/50">`,
		`background: var(--color-white);`,
		`<p class="border-black">`,
		`<div class="shadow-[0_1px_2px_rgba(0,0,0,.1)]">`,
		`box-shadow: 0 12px 26px -10px rgb(0 0 0 / 0.7);`,
		`color: hsl(210 40% 50%);`,
		`<div class="relative border-b border-rule bg-[#f9f7f3] pt-6">`,
		`<meta name="theme-color" content="#ffffff">`,
		`stroke: #abc;`,
		`fill="#1b191780"`,
	} {
		if len(literalColours("page.html.tmpl", line)) == 0 {
			t.Errorf("the guard misses the literal in %q", line)
		}
	}
	for _, line := range []string{
		`<span class="whitespace-nowrap text-on-fill">`,
		`white-space: nowrap;`,
		`<div class="bg-canvas">`,
		`<div class="shadow-(--shadow-field)">`,
		`box-shadow: inset 0 0 0 1px var(--color-crimson-deep), var(--shadow-field);`,
		`color: rgb(from var(--color-ink) r g b / 50%);`,
		`<a href="#add">Add</a>`,
		`<path fill="url(#fade)"/>`,
		`<span>&#160;</span>`,
		`<a href="#main">`,
		`{{with .BarColour}}<meta name="theme-color" content="{{.}}">{{end}}`,
	} {
		if hits := literalColours("page.html.tmpl", line); len(hits) > 0 {
			t.Errorf("the guard flags %v in %q, which is no colour literal", hits, line)
		}
	}
	// A literal kept on purpose is kept in its own file only.
	if hits := literalColours("scroll-edge.css", `mask-image: linear-gradient(to left, transparent, #000 var(--verso-scroll-edge));`); len(hits) > 0 {
		t.Errorf("the scroll edge's mask is flagged: %v", hits)
	}
	if hits := literalColours("field.css", `color: #000;`); len(hits) == 0 {
		t.Error("a #000 outside the mask's file passes as if it were the mask")
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

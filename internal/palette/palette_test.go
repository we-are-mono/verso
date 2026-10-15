// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package palette

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `/* The palette. */
@theme {
  /* Sand — the ground. */
  --color-ground: #faf9f7; /* the page, and every card */
  --color-ink: #1B1917;
  --color-choice: var(--color-denim);
  --shadow-dialog: 0 8px 32px rgba(27, 25, 23, .16);
}

[data-verso-off] {
  --color-green: var(--color-glyph);
  --color-ground: #000000;
}
`

func TestParseReadsEveryColourOfTheTheme(t *testing.T) {
	p, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"ground": "#faf9f7", "ink": "#1b1917"} {
		if got, ok := p.Colour(name); !ok || got != want {
			t.Errorf("Colour(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestParseLeavesOutWhatIsNotAColourValue(t *testing.T) {
	p, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	// A reference is a job handed to another token, a shadow is not a colour,
	// and the switched-off block re-points tokens inside a row: none of them
	// is the palette's own value.
	for _, name := range []string{"choice", "green", "dialog"} {
		if got, ok := p.Colour(name); ok {
			t.Errorf("Colour(%q) = %q; want no colour", name, got)
		}
	}
	if got, _ := p.Colour("ground"); got != "#faf9f7" {
		t.Errorf("Colour(ground) = %q; the switched-off block must not override the theme", got)
	}
}

const withRoot = sample + `
:root {
  --shadow-dialog: 0 8px 32px var(--color-shadow-dialog);
  --color-shadow-dialog: #1b191729; /* a dialog's shadow */
  --color-chart-denim-from: color-mix(in srgb, var(--color-denim) 16%, transparent);
}
`

func TestTokenNamesAreEveryColourATokenHolds(t *testing.T) {
	names := TokenNames(withRoot)
	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}
	// A theme may set any colour token: one the theme block holds as a value
	// or as a reference, and one the plain :root block holds as a hex value
	// or a mix.
	for _, want := range []string{"ground", "ink", "choice", "shadow-dialog", "chart-denim-from"} {
		if !has[want] {
			t.Errorf("TokenNames lacks %q: %v", want, names)
		}
	}
	// green is re-pointed only inside a switched-off row, and dialog is a
	// shadow, not a colour.
	for _, gone := range []string{"green", "dialog"} {
		if has[gone] {
			t.Errorf("TokenNames carries %q, which is no token a theme sets", gone)
		}
	}
	if got, ok := MustParse(t, withRoot).Colour("shadow-dialog"); ok {
		t.Errorf("Colour(shadow-dialog) = %q; only the theme block's values are the palette's colours", got)
	}
}

func TestTokenNamesLeaveOutUnsetTokens(t *testing.T) {
	for _, n := range TokenNames("@theme {\n  --color-ink: #1b1917;\n  --color-white: initial;\n}\n") {
		if n == "white" {
			t.Error("TokenNames carries white, which the palette unsets")
		}
	}
}

// MustParse parses src or fails the test.
func MustParse(t *testing.T, src string) Palette {
	t.Helper()
	p, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseRefusesASourceWithNoTheme(t *testing.T) {
	for _, src := range []string{"", "/* nothing */", "@theme {\n}\n"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded; want an error", src)
		}
	}
}

// TestTheShippedPaletteParses reads the palette the shell embeds, so a change
// to palette.css that the parser cannot read fails here and not at startup.
func TestTheShippedPaletteParses(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "server", "assets", "palette.css"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"canvas": "#ffffff", "ground": "#faf9f7", "hw-tip": "#0b0f16"} {
		if got, ok := p.Colour(name); !ok || got != want {
			t.Errorf("Colour(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	names := map[string]bool{}
	for _, n := range TokenNames(string(src)) {
		names[n] = true
	}
	for _, want := range []string{"canvas", "on-fill", "choice", "choice-border", "hw-tip"} {
		if !names[want] {
			t.Errorf("the shipped palette's TokenNames lacks %q", want)
		}
	}
	if names["white"] || names["black"] {
		t.Error("the shipped palette's TokenNames carries Tailwind's unset white or black")
	}
}

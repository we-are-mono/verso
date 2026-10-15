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
}

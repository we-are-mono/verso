// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"slices"
	"strings"
	"testing"
)

// Every shadow the shell casts keeps its shape in the shell and takes its
// colour from a palette token, one per job, the colour the shadow cast before
// written as a hex with alpha; the page reads both through var(), so a theme
// sets a shadow's colour and nothing else of it.
func TestEveryShadowIsAShapeOverAPaletteColour(t *testing.T) {
	for _, decl := range []string{
		"--shadow-field: inset 0 1px 2px var(--color-shadow-field);",
		"--shadow-float: 0 4px 12px var(--color-shadow-float);",
		"--shadow-menu: 0 8px 24px var(--color-shadow-menu);",
		"--shadow-dialog: 0 8px 32px var(--color-shadow-dialog);",
		"--shadow-drawer: -8px 0 32px var(--color-shadow-drawer);",
		"--shadow-edge: inset 0 4px 4px -4px var(--color-shadow-edge);",
		"--shadow-lifted: 0 1.25rem 2.5rem -0.75rem var(--color-shadow-lifted), 0 .5rem 1rem -.5rem var(--color-shadow-lifted-near);",
		"--shadow-port-tip: 0 12px 26px -10px var(--color-shadow-port-tip);",
		"--color-shadow-field: #1b19170f;",
		"--color-shadow-float: #1b191724;",
		"--color-shadow-menu: #1b191724;",
		"--color-shadow-dialog: #1b191729;",
		"--color-shadow-drawer: #1b19171f;",
		"--color-shadow-edge: #1b191724;",
		"--color-shadow-lifted: #1b19173d;",
		"--color-shadow-lifted-near: #1b19172e;",
		"--color-shadow-port-tip: #000000b3;",
	} {
		if !strings.Contains(paletteText, decl) {
			t.Errorf("palette.css does not declare %s", decl)
		}
		name := decl[:strings.Index(decl, ":")]
		if !strings.Contains(cssText, name+":") {
			t.Errorf("the stylesheet does not carry %s", name)
		}
		if !strings.Contains(cssText, "var("+name+")") {
			t.Errorf("nothing in the stylesheet reads %s", name)
		}
	}
}

// A chart series' wash runs between two palette colours, each by default the
// series' own hue at the strength the wash had, so a theme that moves the hue
// moves its wash, and one that wants another strength sets a colour.
func TestAChartsWashIsAPairOfPaletteColours(t *testing.T) {
	for _, decl := range []string{
		"--color-chart-denim-from: color-mix(in srgb, var(--color-denim) 16%, transparent);",
		"--color-chart-denim-to: color-mix(in srgb, var(--color-denim) 4%, transparent);",
		"--color-chart-amethyst-from: color-mix(in srgb, var(--color-amethyst) 16%, transparent);",
		"--color-chart-amethyst-to: color-mix(in srgb, var(--color-amethyst) 4%, transparent);",
		"--color-chart-violet-from: color-mix(in srgb, var(--color-amethyst) 24%, transparent);",
		"--color-chart-violet-to: transparent;",
		"--color-chart-emerald-from: color-mix(in srgb, var(--color-green) 24%, transparent);",
		"--color-chart-emerald-to: transparent;",
	} {
		if !strings.Contains(paletteText, decl) {
			t.Errorf("palette.css does not declare %s", decl)
		}
		name := decl[:strings.Index(decl, ":")]
		if !strings.Contains(cssText, "stop-color:var("+name+")") {
			t.Errorf("no chart stop reads %s", name)
		}
	}
	// The property, not the row marker's mask, whose SVG gradient keeps its
	// own stop-opacity attributes.
	for _, gone := range []string{"--chart-wash-", "--chart-role-wash-", "stop-opacity:"} {
		if strings.Contains(cssText, gone) {
			t.Errorf("the stylesheet still carries %q: a wash is a colour", gone)
		}
	}
}

// White did two jobs, a surface and a label on a fill, which only agree while
// the page is light. The surface is Canvas and the label On-fill (palette), and
// Tailwind's own white and black are unset, so a white nobody chose compiles to
// nothing.
func TestTheStylesheetSplitsWhitesTwoJobs(t *testing.T) {
	if !strings.Contains(cssText, "--color-on-fill:#fff") {
		t.Error("the stylesheet does not define --color-on-fill as white")
	}
	for _, gone := range []string{"--color-white:", "--color-black:", ".text-white", ".bg-white", "-white\\/"} {
		if strings.Contains(cssText, gone) {
			t.Errorf("the stylesheet still carries %q", gone)
		}
	}
}

// The shell reads its own palette at startup, so what it writes outside the
// stylesheet (a takeover's browser bar colour) is the palette's value.
func TestTheServerCarriesThePalette(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for name, want := range map[string]string{"canvas": "#ffffff", "ground": "#faf9f7"} {
		if got, ok := s.palette.Colour(name); !ok || got != want {
			t.Errorf("palette %q = %q, %v; want %q", name, got, ok, want)
		}
	}
	// The keys a theme may set: every colour token, the shadow colours and
	// chart washes among them.
	if !slices.Contains(s.tokens, "shadow-dialog") || !slices.Contains(s.tokens, "chart-denim-from") || !slices.Contains(s.tokens, "ink") {
		t.Errorf("the server's theme keys lack a colour token: %v", s.tokens)
	}
}

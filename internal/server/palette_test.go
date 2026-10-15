// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"
)

// Every shadow the shell casts is a palette token, one per job, holding the
// shadow it replaced; the page reads it through var(), so the token is the one
// place the shadow is decided.
func TestEveryShadowIsAPaletteToken(t *testing.T) {
	for _, decl := range []string{
		"--shadow-field: inset 0 1px 2px rgba(27, 25, 23, .06);",
		"--shadow-float: 0 4px 12px rgba(27, 25, 23, .14);",
		"--shadow-menu: 0 8px 24px rgba(27, 25, 23, .14);",
		"--shadow-dialog: 0 8px 32px rgba(27, 25, 23, .16);",
		"--shadow-drawer: -8px 0 32px rgba(27, 25, 23, .12);",
		"--shadow-edge: inset 0 4px 4px -4px rgba(27, 25, 23, .14);",
		"--shadow-lifted: 0 1.25rem 2.5rem -0.75rem rgb(27 25 23 / .24), 0 .5rem 1rem -.5rem rgb(27 25 23 / .18);",
		"--shadow-port-tip: 0 12px 26px -10px rgb(0 0 0 / .7);",
	} {
		if !strings.Contains(paletteText, decl) {
			t.Errorf("palette.css does not declare %s", decl)
		}
		name := decl[:strings.Index(decl, ":")]
		if !strings.Contains(cssText, name+":") {
			t.Errorf("the stylesheet does not carry %s", name)
		}
		if !strings.Contains(cssText, "var("+name+")") {
			t.Errorf("nothing in the stylesheet casts %s", name)
		}
	}
}

// A chart series' wash runs between two palette tokens, for the live traffic
// series and for a role series, each holding the strengths it replaced.
func TestAChartsWashIsAPaletteToken(t *testing.T) {
	for _, decl := range []string{
		"--chart-wash-from: .16;",
		"--chart-wash-to: .04;",
		"--chart-role-wash-from: .24;",
		"--chart-role-wash-to: 0;",
	} {
		if !strings.Contains(paletteText, decl) {
			t.Errorf("palette.css does not declare %s", decl)
		}
		name := decl[:strings.Index(decl, ":")]
		if !strings.Contains(cssText, "var("+name+")") {
			t.Errorf("no chart stop reads %s", name)
		}
	}
	if strings.Contains(cssText, "stop-opacity:.") || strings.Contains(cssText, "stop-opacity:0}") {
		t.Error("a chart stop still writes its strength as a number")
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
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"reflect"
	"testing"

	"github.com/we-are-mono/verso/internal/palette"
)

var (
	sandDark = Theme{ID: SandDark, Name: "Sand", Family: "Sand", Scheme: Dark,
		Colors: map[string]string{"ink": "#eeeae3", "ground": "#1c1a17", "canvas": "#121110"}}
	mocha = Theme{ID: "catppuccin-mocha", Name: "Mocha", Family: "Catppuccin", Scheme: Dark,
		Colors: map[string]string{"ground": "#1e1e2e", "canvas": "#11111b"}}
	latte = Theme{ID: "catppuccin-latte", Name: "Latte", Family: "Catppuccin", Scheme: Light,
		Colors: map[string]string{"ground": "#eff1f5"}}
	themes = Set{SandDark: sandDark, mocha.ID: mocha, latte.ID: latte}
	base   = palette.Palette{"canvas": "#ffffff", "ground": "#faf9f7"}
)

func TestCSSForEachChoice(t *testing.T) {
	sand := "@media (prefers-color-scheme: dark){:root{color-scheme:dark;--color-canvas:#121110;--color-ground:#1c1a17;--color-ink:#eeeae3;}}"
	for _, tc := range []struct{ choice, css string }{
		{"", sand},
		{Sand, sand},
		{SandLight, ""},
		{SandDark, ":root{color-scheme:dark;--color-canvas:#121110;--color-ground:#1c1a17;--color-ink:#eeeae3;}"},
		{mocha.ID, ":root{color-scheme:dark;--color-canvas:#11111b;--color-ground:#1e1e2e;}"},
		{latte.ID, ":root{color-scheme:light;--color-ground:#eff1f5;}"},
	} {
		css, known := themes.CSS(tc.choice)
		if css != tc.css || !known {
			t.Errorf("CSS(%q) = %q, %v; want %q, true", tc.choice, css, known, tc.css)
		}
	}
}

func TestAnUnknownChoiceIsSandFollowingTheBrowser(t *testing.T) {
	want, _ := themes.CSS(Sand)
	css, known := themes.CSS("gone")
	if css != want || known {
		t.Errorf("CSS(gone) = %q, %v; want Sand's, false", css, known)
	}
}

func TestSandWithoutItsDarkFileIsLightInBoth(t *testing.T) {
	css, known := Set{}.CSS(Sand)
	if css != "" || !known {
		t.Errorf("CSS(sand) without sand-dark = %q, %v; want nothing, true", css, known)
	}
}

func TestBarColoursForEachChoice(t *testing.T) {
	for _, tc := range []struct {
		choice, token string
		want          []Bar
	}{
		{Sand, "ground", []Bar{{"(prefers-color-scheme: light)", "#faf9f7"}, {"(prefers-color-scheme: dark)", "#1c1a17"}}},
		{SandLight, "canvas", []Bar{{"", "#ffffff"}}},
		{SandDark, "canvas", []Bar{{"", "#121110"}}},
		{mocha.ID, "canvas", []Bar{{"", "#11111b"}}},
		// A theme that leaves the token to the palette declares the palette's.
		{latte.ID, "canvas", []Bar{{"", "#ffffff"}}},
		{"gone", "ground", []Bar{{"(prefers-color-scheme: light)", "#faf9f7"}, {"(prefers-color-scheme: dark)", "#1c1a17"}}},
	} {
		if got := themes.BarColours(tc.choice, tc.token, base); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("BarColours(%q, %q) = %v; want %v", tc.choice, tc.token, got, tc.want)
		}
	}
	if got := (Set{}).BarColours(Sand, "ground", base); !reflect.DeepEqual(got, []Bar{{"", "#faf9f7"}}) {
		t.Errorf("BarColours(sand) without sand-dark = %v; want Paper alone", got)
	}
}

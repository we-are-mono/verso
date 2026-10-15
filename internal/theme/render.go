// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"strings"

	"github.com/we-are-mono/verso/internal/palette"
)

// Bar is one browser bar colour a page declares (theme-color), under the media
// query it holds for, or for every browser when Media is empty.
type Bar struct {
	Media  string
	Colour string
}

const (
	prefersLight = "(prefers-color-scheme: light)"
	prefersDark  = "(prefers-color-scheme: dark)"
)

// CSS is the style a page carries after the stylesheet for choice: nothing
// for Sand light, which the stylesheet already is; Sand dark under a dark
// preference for Sand following the browser; and a fixed theme's colours and
// scheme for any other theme. A choice no theme answers renders as Sand
// following the browser, and known says so. The colours were checked as hex
// when the theme loaded, so nothing but a colour reaches the style.
func (s Set) CSS(choice string) (css string, known bool) {
	t, follows, known := s.resolve(choice)
	switch {
	case t == nil:
		return "", known
	case follows:
		return "@media " + prefersDark + "{" + t.root() + "}", known
	default:
		return t.root(), known
	}
}

// BarColours are the bar colours a page standing on token (canvas or ground)
// declares under choice: one per preference while Sand follows the browser,
// one otherwise, each the theme's value for token or, where the theme leaves
// it, the palette's.
func (s Set) BarColours(choice, token string, base palette.Palette) []Bar {
	light, _ := base.Colour(token)
	t, follows, _ := s.resolve(choice)
	switch {
	case t == nil:
		return []Bar{{Colour: light}}
	case follows:
		return []Bar{{prefersLight, light}, {prefersDark, t.colour(token, light)}}
	default:
		return []Bar{{Colour: t.colour(token, light)}}
	}
}

// resolve is the theme choice draws in, whether it follows the browser (Sand
// dark under a dark preference only), and whether choice named a theme. No
// theme at all is Sand light, the palette itself.
func (s Set) resolve(choice string) (t *Theme, follows, known bool) {
	switch choice {
	case SandLight:
		return nil, false, true
	case "", Sand:
		return s.dark(), true, true
	}
	if found, ok := s[choice]; ok {
		return &found, false, true
	}
	return s.dark(), true, false
}

// dark is Sand dark, or nil when its file is not there.
func (s Set) dark() *Theme {
	if t, ok := s[SandDark]; ok {
		return &t
	}
	return nil
}

// root is the theme as one :root rule: its scheme, then its colours.
func (t Theme) root() string {
	var b strings.Builder
	b.WriteString(":root{color-scheme:")
	b.WriteString(string(t.Scheme))
	b.WriteString(";")
	for _, k := range t.keys() {
		b.WriteString("--color-")
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(t.Colors[k])
		b.WriteString(";")
	}
	b.WriteString("}")
	return b.String()
}

// colour is the theme's value for token, or fallback where it sets none.
func (t Theme) colour(token, fallback string) string {
	if v, ok := t.Colors[token]; ok {
		return v
	}
	return fallback
}

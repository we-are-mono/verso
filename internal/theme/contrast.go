// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/palette"
)

// pair is a colour that carries words or a glyph, the ground it stands on,
// and the contrast it keeps there (the palette's 4.5 Floor, DESIGN.md).
type pair struct {
	fg, bg string
	floor  float64
}

// pairs are the pairings the palette's jobs promise. A mark the palette keeps
// under 3 : 1 on purpose (the marigold square) carries no words and is not
// among them.
var pairs = []pair{
	{"ink", "ground", 4.5}, {"ink", "canvas", 4.5},
	{"body", "ground", 4.5}, {"body", "quiet", 4.5}, {"body", "mid", 4.5},
	{"meta", "ground", 4.5}, {"meta", "quiet", 4.5}, {"meta", "canvas", 4.5},
	// Meta on Mid Sand is the one pairing Sand light keeps a hair under 4.5,
	// at 4.4 : 1 (palette.css); a theme is held to the same.
	{"meta", "mid", 4.35},
	{"on-fill", "denim", 4.5}, {"on-fill", "denim-deep", 4.5},
	{"on-fill", "crimson", 4.5}, {"on-fill", "marigold-deep", 4.5},
	{"denim-deep", "denim-soft", 4.5}, {"green-deep", "green-soft", 4.5},
	{"marigold-deep", "marigold-soft", 4.5}, {"crimson-deep", "crimson-soft", 4.5},
	{"amethyst-deep", "amethyst-soft", 4.5},
	{"glyph", "ground", 3},
}

// shadows are the shadow colours a dark theme is expected to darken: Ink at a
// light theme's strengths barely shows on a dark ground.
var shadows = []string{"shadow-field", "shadow-float", "shadow-menu", "shadow-dialog", "shadow-drawer", "shadow-edge"}

// Contrast is each place the theme, laid over base, falls short: a pairing
// under its floor, and a dark theme that leaves every shadow colour as the
// light palette has it. Empty when the theme keeps every floor.
func (t Theme) Contrast(base palette.Palette) []string {
	colours := map[string]string{}
	for k, v := range base {
		colours[k] = v
	}
	for k, v := range t.Colors {
		colours[k] = v
	}
	var short []string
	for _, p := range pairs {
		fg, bg, ok := solid(colours, p.fg, p.bg)
		if !ok {
			continue
		}
		if r := ratio(fg, bg); r < p.floor {
			short = append(short, fmt.Sprintf("%s: %s on %s stands at %.2f : 1, under %g : 1", t.ID, p.fg, p.bg, r, p.floor))
		}
	}
	if t.Scheme == Dark && !setsAny(t.Colors, shadows) {
		short = append(short, fmt.Sprintf("%s: a dark theme that keeps the light palette's shadow colours casts shadows that barely show", t.ID))
	}
	return short
}

// setsAny is whether colors sets any of keys.
func setsAny(colors map[string]string, keys []string) bool {
	for _, k := range keys {
		if _, ok := colors[k]; ok {
			return true
		}
	}
	return false
}

// rgba is a colour's channels, 0 to 1.
type rgba struct{ r, g, b, a float64 }

// solid is the pair's two colours as they meet the eye: a translucent ground
// over the page's ground, then a translucent colour over that. A pairing whose
// colours the palette holds as no hex value (a reference, a mix) is not read.
func solid(colours map[string]string, fg, bg string) (rgba, rgba, bool) {
	f, okF := parse(colours[fg])
	b, okB := parse(colours[bg])
	page, okP := parse(colours["ground"])
	if !okF || !okB || !okP {
		return rgba{}, rgba{}, false
	}
	b = over(b, page)
	return over(f, b), b, true
}

// over lays c over an opaque ground.
func over(c, ground rgba) rgba {
	mix := func(x, y float64) float64 { return x*c.a + y*(1-c.a) }
	return rgba{mix(c.r, ground.r), mix(c.g, ground.g), mix(c.b, ground.b), 1}
}

// parse reads a hex colour of 3, 4, 6 or 8 digits.
func parse(v string) (rgba, bool) {
	v = strings.TrimPrefix(v, "#")
	if len(v) == 3 || len(v) == 4 {
		var b strings.Builder
		for _, c := range v {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		v = b.String()
	}
	if len(v) == 6 {
		v += "ff"
	}
	if len(v) != 8 {
		return rgba{}, false
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return rgba{}, false
	}
	ch := func(shift uint) float64 { return float64(n>>shift&0xff) / 255 }
	return rgba{ch(24), ch(16), ch(8), ch(0)}, true
}

// ratio is the WCAG contrast ratio of two opaque colours.
func ratio(a, b rgba) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// luminance is a colour's relative luminance (WCAG 2.2).
func luminance(c rgba) float64 {
	lin := func(x float64) float64 {
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

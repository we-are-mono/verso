// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package palette reads the shell's palette (palette.css) as Go sees it: the
// colour each named token holds. The stylesheet is the one place a colour is
// written; what the shell writes outside it (a page's browser bar colour)
// reads the value from here rather than keeping a copy.
package palette

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

// Palette is the palette's colour tokens by name, "ground" for --color-ground.
type Palette map[string]string

var (
	comment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	theme   = regexp.MustCompile(`@theme\s*\{([^}]*)\}`)
	colour  = regexp.MustCompile(`--color-([a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;`)
	// root is a plain :root block standing at the top level, where the
	// palette keeps the tokens Tailwind does not make utilities of.
	root  = regexp.MustCompile(`(?m)^:root\s*\{([^}]*)\}`)
	token = regexp.MustCompile(`--color-([a-z0-9-]+)\s*:\s*([^;]+);`)
)

// TokenNames is every colour token the palette declares, by name ("ground"
// for --color-ground), in its @theme block and its top-level :root blocks:
// the colours a theme may set. A token holding another's value or a mix is
// one, since a theme may give it its own; a token the palette unsets
// (initial) is not, and a switched-off row's re-pointing is no token at all.
func TokenNames(src string) []string {
	src = comment.ReplaceAllString(src, "")
	var blocks []string
	if m := theme.FindStringSubmatch(src); m != nil {
		blocks = append(blocks, m[1])
	}
	for _, m := range root.FindAllStringSubmatch(src, -1) {
		blocks = append(blocks, m[1])
	}
	seen := map[string]bool{}
	var names []string
	for _, block := range blocks {
		for _, m := range token.FindAllStringSubmatch(block, -1) {
			if strings.TrimSpace(m[2]) == "initial" || seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	sort.Strings(names)
	return names
}

// Parse reads the colour tokens of src's @theme block. A token that hands its
// job to another (var(--color-…)) is not a value of its own and is left out,
// as is anything outside @theme. A source with no colour in its @theme is an
// error, since the shell cannot draw without one.
func Parse(src string) (Palette, error) {
	block := theme.FindStringSubmatch(comment.ReplaceAllString(src, ""))
	if block == nil {
		return nil, errors.New("palette: no @theme block")
	}
	p := Palette{}
	for _, m := range colour.FindAllStringSubmatch(block[1], -1) {
		p[m[1]] = strings.ToLower(m[2])
	}
	if len(p) == 0 {
		return nil, errors.New("palette: no colour in @theme")
	}
	return p, nil
}

// Colour is the value the named token holds, and whether the palette has it.
func (p Palette) Colour(name string) (string, bool) {
	v, ok := p[name]
	return v, ok
}

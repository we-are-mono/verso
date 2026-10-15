// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package theme reads the router's themes (ADR-024): each a file of colours
// keyed by palette token, laid over the palette Sand light is. It validates a
// file before any of it can reach a page, renders the theme in force as the
// custom properties a page carries after the stylesheet, and says where a
// theme's words fall under the contrast the palette keeps.
package theme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Scheme is whether a theme is light or dark, which the page declares so the
// browser's own controls and scrollbars follow it.
type Scheme string

// The two schemes.
const (
	Light Scheme = "light"
	Dark  Scheme = "dark"
)

// The built-in choices. Sand follows the browser between Sand light, which is
// the palette itself, and Sand dark, which ships as a file under its own id.
const (
	Sand      = "sand"
	SandLight = "sand-light"
	SandDark  = "sand-dark"
)

// Theme is one palette: its id (the file's name), the name and family the
// Appearance card shows, its scheme, and the colours it sets, by token name.
type Theme struct {
	ID     string
	Name   string
	Family string
	Scheme Scheme
	Colors map[string]string
}

// Set is the loaded themes by id.
type Set map[string]Theme

var (
	// id is a theme's id, its file's name: what the UCI option holds.
	id = regexp.MustCompile(`^[a-z0-9-]+$`)
	// hex is the one form a theme's colour takes. It is what keeps a value from
	// writing anything but a colour into the page's style block.
	hex = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// document is a theme file as it is written.
type document struct {
	Name   string            `json:"name"`
	Family string            `json:"family"`
	Scheme Scheme            `json:"scheme"`
	Colors map[string]string `json:"colors"`
}

// Load reads every *.json in fsys as a theme whose keys must be among names,
// the palette's colour tokens. A file that breaks the format is left out and
// reported, naming the file and the cause; the rest load.
func Load(fsys fs.FS, names []string) (Set, []error) {
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	paths, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return Set{}, []error{fmt.Errorf("themes: %w", err)}
	}
	set := Set{}
	var problems []error
	for _, p := range paths {
		t, err := read(fsys, p, known)
		if err != nil {
			problems = append(problems, fmt.Errorf("theme %s: %w", p, err))
			continue
		}
		set[t.ID] = t
	}
	return set, problems
}

// read reads and checks one theme file.
func read(fsys fs.FS, p string, known map[string]bool) (Theme, error) {
	name := strings.TrimSuffix(path.Base(p), ".json")
	switch {
	case !id.MatchString(name):
		return Theme{}, fmt.Errorf("%q is not an id: a theme's file is named in lower-case letters, digits and hyphens", name)
	case name == Sand || name == SandLight:
		return Theme{}, fmt.Errorf("%q is a built-in theme's id", name)
	}
	raw, err := fs.ReadFile(fsys, p)
	if err != nil {
		return Theme{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var f document
	if err := dec.Decode(&f); err != nil {
		return Theme{}, err
	}
	switch {
	case strings.TrimSpace(f.Name) == "":
		return Theme{}, fmt.Errorf("it has no name")
	case f.Scheme != Light && f.Scheme != Dark:
		return Theme{}, fmt.Errorf("its scheme %q is neither light nor dark", f.Scheme)
	}
	colors := make(map[string]string, len(f.Colors))
	for key, value := range f.Colors {
		if !known[key] {
			return Theme{}, fmt.Errorf("the palette has no colour %q", key)
		}
		if !hex.MatchString(value) {
			return Theme{}, fmt.Errorf("%s is %q, not a hex colour of 3, 4, 6 or 8 digits", key, value)
		}
		colors[key] = strings.ToLower(value)
	}
	return Theme{ID: name, Name: f.Name, Family: f.Family, Scheme: f.Scheme, Colors: colors}, nil
}

// keys is a theme's colour names in order, so its rendering never changes
// from one render to the next.
func (t Theme) keys() []string {
	keys := make([]string, 0, len(t.Colors))
	for k := range t.Colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

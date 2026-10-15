// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"strings"
	"testing"
	"testing/fstest"
)

// names are the colour tokens these tests' palette declares.
var names = []string{"canvas", "ground", "quiet", "mid", "ink", "body", "meta", "glyph", "denim", "on-fill", "shadow-dialog"}

func file(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

func TestLoadReadsAValidTheme(t *testing.T) {
	set, problems := Load(fstest.MapFS{
		"catppuccin-mocha.json": file(`{"name":"Mocha","family":"Catppuccin","scheme":"dark",
			"colors":{"ground":"#1E1E2E","ink":"#cdd6f4","shadow-dialog":"#00000080","denim":"#89b"}}`),
	}, names)
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	got, ok := set["catppuccin-mocha"]
	if !ok {
		t.Fatalf("the theme is not loaded under its file's name: %v", set)
	}
	if got.Name != "Mocha" || got.Family != "Catppuccin" || got.Scheme != Dark {
		t.Errorf("theme = %+v", got)
	}
	// Values read as lower case, whatever their length.
	for key, want := range map[string]string{"ground": "#1e1e2e", "shadow-dialog": "#00000080", "denim": "#89b"} {
		if got.Colors[key] != want {
			t.Errorf("Colors[%q] = %q; want %q", key, got.Colors[key], want)
		}
	}
}

func TestLoadTakesEveryHexLength(t *testing.T) {
	for _, v := range []string{"#abc", "#abcd", "#aabbcc", "#aabbccdd"} {
		_, problems := Load(fstest.MapFS{"t.json": file(`{"name":"T","scheme":"light","colors":{"ink":"` + v + `"}}`)}, names)
		if len(problems) > 0 {
			t.Errorf("%s refused: %v", v, problems)
		}
	}
}

func TestLoadSkipsAThemeThatBreaksTheFormat(t *testing.T) {
	for _, tc := range []struct{ name, file, body, cause string }{
		{"not JSON", "a.json", `{"name":`, "a.json"},
		{"no name", "b.json", `{"scheme":"dark","colors":{}}`, "name"},
		{"no scheme", "c.json", `{"name":"C","colors":{}}`, "scheme"},
		{"an unknown scheme", "d.json", `{"name":"D","scheme":"dim","colors":{}}`, "dim"},
		{"an unknown key", "e.json", `{"name":"E","scheme":"dark","colors":{"denium":"#000"}}`, "denium"},
		{"a value leaving the stylesheet", "f.json", `{"name":"F","scheme":"dark","colors":{"ground":"red;}body{display:none"}}`, "ground"},
		{"a colour function", "g.json", `{"name":"G","scheme":"dark","colors":{"ground":"rgb(0,0,0)"}}`, "ground"},
		{"a hex of five digits", "h.json", `{"name":"H","scheme":"dark","colors":{"ground":"#12345"}}`, "ground"},
		{"an unknown field", "i.json", `{"name":"I","scheme":"dark","colours":{}}`, "colours"},
		{"the built-in Sand's id", "sand.json", `{"name":"S","scheme":"light","colors":{}}`, "sand"},
		{"the built-in Sand light's id", "sand-light.json", `{"name":"S","scheme":"light","colors":{}}`, "sand-light"},
		{"an id that is no name", "My Theme.json", `{"name":"M","scheme":"light","colors":{}}`, "My Theme"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, problems := Load(fstest.MapFS{
				tc.file:     file(tc.body),
				"good.json": file(`{"name":"Good","scheme":"light","colors":{"ink":"#000"}}`),
			}, names)
			if len(problems) != 1 {
				t.Fatalf("problems = %v; want one", problems)
			}
			msg := problems[0].Error()
			if !strings.Contains(msg, tc.file) || !strings.Contains(msg, tc.cause) {
				t.Errorf("problem %q names neither the file %q nor the cause %q", msg, tc.file, tc.cause)
			}
			if len(set) != 1 {
				t.Errorf("set = %v; want only the good theme", set)
			}
		})
	}
}

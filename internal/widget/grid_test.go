// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRenderGrid(t *testing.T) {
	r := newRenderer(t)
	g := &Grid{Columns: 2, Children: []Widget{
		&Stat{Label: "A", Value: "1"},
		&Stat{Label: "B", Value: "2"},
	}}
	got := render(t, r, g)
	if !strings.Contains(got, "grid-cols-2") {
		t.Errorf("two-column grid classes missing: %s", got)
	}
	for _, want := range []string{"A", "B"} {
		if !strings.Contains(got, want) {
			t.Errorf("grid child %q not rendered: %s", want, got)
		}
	}
}

// TestGridColumnsResponsive checks the count→responsive-class mapping the shell owns.
func TestGridColumnsResponsive(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{1: "grid-cols-1", 2: "grid-cols-2", 3: "md:grid-cols-3", 4: "md:grid-cols-4"}
	for cols, want := range cases {
		got := render(t, r, &Grid{Columns: cols, Children: []Widget{&Stat{Label: "x", Value: "1"}}})
		if !strings.Contains(got, want) {
			t.Errorf("columns=%d: want class %q in:\n%s", cols, want, got)
		}
	}
}

func TestRenderFormGrid(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 3, Children: []Widget{
		&Field{Name: "password", Label: "New password", Kind: "password"},
		&Field{Name: "confirm", Label: "Repeat password", Kind: "password"},
	}})
	for _, want := range []string{"grid-cols-1 md:grid-cols-3", "gap-6", "New password", "Repeat password"} {
		if !strings.Contains(got, want) {
			t.Errorf("form grid missing %q:\n%s", want, got)
		}
	}
}

// TestDecodeGridChildren covers UnmarshalJSON: a grid decodes its children through
// the shared Decode, so an unknown child fails loudly rather than vanishing.
func TestDecodeGridChildren(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"grid","columns":3,"children":[
		{"type":"stat","label":"Devices","value":"9"},
		{"type":"badge","variant":"success","text":"up"}
	]}`))
	if err != nil {
		t.Fatalf("decode grid: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"md:grid-cols-3", "Devices", "up"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded grid missing %q:\n%s", want, got)
		}
	}

	if _, err := Decode([]byte(`{"type":"grid","children":[{"type":"nope"}]}`)); err == nil {
		t.Error("grid with an unknown child should fail to decode, not vanish")
	}
}

// TestGridRail: a rail is the page's work beside a narrower column that comments
// on it — a fixed reading measure and a fixed rail, not an even split, with the
// page's own side padding as the gutter between them. The two columns are the
// whole of the wide page, so they are keyed to the grid's own container rather
// than to the window; narrower than that the rail stacks under the work, where
// two columns would leave neither readable.
func TestGridRail(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{
		Style: "rail", Columns: 2,
		Children: []Widget{&Callout{Body: "the work"}, &Callout{Body: "the rail"}},
	})
	for _, want := range []string{
		`<div class="@container">`,
		"grid grid-cols-1 gap-10 @6xl:grid-cols-[40rem_29.25rem]",
		"the work", "the rail",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rail grid missing %q:\n%s", want, got)
		}
	}
	// A rail is never the even split an ordinary grid draws.
	if strings.Contains(got, "md:grid-cols-2") {
		t.Errorf("a rail must not fall back to even columns:\n%s", got)
	}
}

// TestGridColumnsReachTheStylesheet: every column class a grid can render is a
// rule in the built stylesheet. The stylesheet is compiled from the templates
// alone, so a class that only Go named was never compiled — and a grid whose
// column class is missing does not fail, it silently draws one column. Render
// every style at every count and look each class up in the CSS as the selector
// Tailwind writes for it.
func TestGridColumnsReachTheStylesheet(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "verso.css"))
	if err != nil {
		t.Fatal(err)
	}
	attr := regexp.MustCompile(`class="([^"]*)"`)
	escape := strings.NewReplacer("@", `\@`, ":", `\:`, "[", `\[`, "]", `\]`, ".", `\.`)
	r := newRenderer(t)
	for _, style := range []string{"", "form", "strip", "rail"} {
		for columns := 1; columns <= 5; columns++ {
			got := render(t, r, &Grid{Style: style, Columns: columns, Children: []Widget{&Callout{Body: "a"}, &Callout{Body: "b"}}})
			for _, m := range attr.FindAllStringSubmatch(got, -1) {
				for _, class := range strings.Fields(m[1]) {
					if !strings.Contains(class, "grid-cols") && !strings.HasPrefix(class, "gap-") && class != "@container" {
						continue
					}
					if sel := "." + escape.Replace(class); !strings.Contains(string(css), sel+"{") && !strings.Contains(string(css), sel+",") {
						t.Errorf("style %q, %d columns: class %q is not in the built stylesheet", style, columns, class)
					}
				}
			}
		}
	}
}

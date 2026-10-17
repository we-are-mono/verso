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
		&Text{Markdown: "A"},
		&Text{Markdown: "B"},
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

// Cards of one kind stand in a row of cells, each card in a cell of its own,
// so none is the card after another (sections.css spaces those) and the row
// lays them side by side where the page has room.
func TestGridCardsStandInCells(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "cards", Children: []Widget{
		&Section{Title: "SFP+ module", Chip: "xfi0", Hairline: true},
		&Section{Title: "SFP+ module", Chip: "xfi1", Hairline: true},
	}})
	if !strings.HasPrefix(strings.TrimSpace(got), `<div data-verso-cards><div>`) {
		t.Errorf("the row should open the cards' cells:\n%s", got)
	}
	if n := strings.Count(got, `<div class="min-w-0"><section`); n != 2 {
		t.Errorf("%d cards in cells of their own, want 2:\n%s", n, got)
	}
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "sections.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "[data-verso-cards] > div {") {
		t.Error("sections.css should lay the cards' row out")
	}
}

// A figure stands beside what is said of it: what is said first, taking the
// row, then the figure at its own width (sections.css lays them out and
// wraps them).
func TestGridFigureStandsBesideItsFacts(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "figure", Children: []Widget{
		&Properties{Align: "left", Items: []Property{{Label: "Memory", Value: "7.7 GiB"}}},
		&Ports{Back: `<svg viewBox="0 0 504 136"></svg>`, Items: []PortItem{{Iface: "eth0"}}},
	}})
	row, list, ports := strings.Index(got, "<div data-verso-beside>"), strings.Index(got, "<dl"), strings.Index(got, `class="verso-ports`)
	if row < 0 || list < row || ports < list {
		t.Errorf("the facts should open the row, then the figure:\n%s", got)
	}
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "sections.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "[data-verso-beside] > :last-child {") {
		t.Error("sections.css should size the figure's column")
	}
	// The row's hook is its own: a listing's live figures (a device's Now
	// cell) carry data-verso-figure, and a rule aimed at that would stack
	// them and stretch every row.
	if strings.Contains(string(css), "[data-verso-figure]") {
		t.Error("sections.css styles the live figures' hook")
	}
	templates, err := filepath.Glob(filepath.Join("templates", "*.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range templates {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "data-verso-beside") && filepath.Base(path) != "grid.html.tmpl" {
			t.Errorf("%s uses the figure row's hook", path)
		}
	}
}

// TestGridColumnsResponsive checks the count→responsive-class mapping the shell owns.
func TestGridColumnsResponsive(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{1: "grid-cols-1", 2: "grid-cols-2", 3: "md:grid-cols-3", 4: "md:grid-cols-4"}
	for cols, want := range cases {
		got := render(t, r, &Grid{Columns: cols, Children: []Widget{&Text{Markdown: "x"}}})
		if !strings.Contains(got, want) {
			t.Errorf("columns=%d: want class %q in:\n%s", cols, want, got)
		}
	}
}

// TestRelatedFieldsStandAsRows: a set of related fields that cannot fuse
// stands as a row per field, never side by side in columns (form-fields):
// unlabelled, the rows join the form's run; labelled, the set's name and
// description head them, as a switch group's do. A set holding something
// that is not a field keeps its columns.
func TestRelatedFieldsStandAsRows(t *testing.T) {
	r := newRenderer(t)
	unlabelled := render(t, r, &Grid{Style: "form", Columns: 3, Children: []Widget{
		&Field{Name: "port", Label: "Port"},
		&List{Name: "servers", Label: "Servers"},
	}})
	if strings.Contains(unlabelled, "verso-form-grid") || strings.Count(unlabelled, `class="verso-field-row`) != 2 {
		t.Errorf("an unlabelled set is a row per field:\n%s", unlabelled)
	}
	labelled := render(t, r, &Grid{Style: "form", Columns: 2, Label: "Timing", Help: "How often it runs.", Children: []Widget{
		&Field{Name: "port", Label: "Port"},
		&List{Name: "servers", Label: "Servers"},
	}})
	set, head, port := strings.Index(labelled, `class="verso-field-set"`), strings.Index(labelled, ">Timing<"), strings.Index(labelled, ">Port<")
	if set < 0 || head < set || port < head || strings.Count(labelled, `class="verso-field-row`) != 3 || strings.Contains(labelled, "verso-form-grid") {
		t.Errorf("a labelled set is its head, then a row per field:\n%s", labelled)
	}
	mixed := render(t, r, &Grid{Style: "form", Columns: 3, Children: []Widget{
		&Field{Name: "port", Label: "Port"},
		&Text{Markdown: "Ports below 1024 need root."},
	}})
	for _, want := range []string{`class="verso-form-grid"`, `data-verso-columns="3"`} {
		if !strings.Contains(mixed, want) {
			t.Errorf("a set holding more than fields keeps its columns, missing %q:\n%s", want, mixed)
		}
	}
}

// TestASecretAndItsRepeatAreOneControl: a new password and its repeat are one
// answer typed twice, so they fuse like any related values — as secrets: never
// reflected, told to the password manager as new, set in words, each part at
// the fused secret's measure.
func TestASecretAndItsRepeatAreOneControl(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 2, Label: "New password", Children: []Widget{
		&Field{Name: "password", Label: "New password", Kind: "password", Autocomplete: "new-password", Value: "hunter22"},
		&Field{Name: "confirm", Label: "Repeat new password", Kind: "password", Value: "hunter22", Error: "The passwords differ."},
	}})
	for _, want := range []string{
		`role="group" aria-labelledby="password-group-label"`,
		`class="verso-box-part" data-verso-measure="secret"`,
		`id="password" name="password" type="password" autocomplete="new-password" aria-label="New password"`,
		`id="confirm" name="confirm" type="password" autocomplete="new-password" aria-label="Repeat new password"`,
		`data-verso-change-kind="password"`,
		"font-sans text-sm",
		"Repeat new password: The passwords differ.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fused secret missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hunter22") {
		t.Errorf("a secret is never reflected into the page:\n%s", got)
	}
}

// TestDecodeGridChildren covers UnmarshalJSON: a grid decodes its children through
// the shared Decode, so an unknown child fails loudly rather than vanishing.
func TestDecodeGridChildren(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"grid","columns":3,"children":[
		{"type":"text","markdown":"Devices"},
		{"type":"text","markdown":"up"}
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
// on it — the form measure and the remaining rail, with the
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
		"grid grid-cols-1 gap-10 @form-split:grid-cols-form",
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

// TestASideColumnStaysInView: beside a long form, the side column (a rail, or
// a settings page's) stays in view as the form scrolls, 32px under the top
// bar's 56px, rather than under the bar. It only does while the columns stand
// side by side.
func TestASideColumnStaysInView(t *testing.T) {
	for _, style := range []string{"rail", "settings"} {
		got := render(t, newRenderer(t), &Grid{
			Style: style, Columns: 2,
			Children: []Widget{&Callout{Body: "the work"}, &Callout{Body: "the side"}},
		})
		for _, want := range []string{"[&>div:last-child]:self-start", "@form-split:[&>div:last-child]:sticky", "@form-split:[&>div:last-child]:top-22"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s grid: the side column is missing %q:\n%s", style, want, got)
			}
		}
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
	for _, style := range []string{"", "form", "rail"} {
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

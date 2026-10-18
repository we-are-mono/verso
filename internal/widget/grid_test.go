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
	for _, want := range []string{
		`<div class="verso-grid" data-verso-columns="2">`,
		`<div class="verso-grid__track">`,
		`<div class="verso-grid__cell">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("two-column grid missing %q: %s", want, got)
		}
	}
	for _, want := range []string{"A", "B"} {
		if !strings.Contains(got, want) {
			t.Errorf("grid child %q not rendered: %s", want, got)
		}
	}
	declares(t, "grid", ".verso-grid__track", "display: grid;", "grid-template-columns: repeat(2, minmax(0, 1fr));")
}

// cssRuleWithin is cssRule for the first rule sel after the at-rule at, or
// "" when the sheet has no such at-rule.
func cssRuleWithin(css, at, sel string) string {
	i := strings.Index(css, at+" {")
	if i < 0 {
		return ""
	}
	return cssRule(css[i:], sel)
}

// The selectors grid.css writes for a split (settings, editor, rail).
const (
	splitTrack = `.verso-grid__track:where(.verso-grid:is([data-verso-style="settings"], [data-verso-style="editor"], [data-verso-style="rail"]) > *)`
	splitSide  = `.verso-grid__cell:last-child:where(.verso-grid:is([data-verso-style="settings"], [data-verso-style="editor"], [data-verso-style="rail"]) *)`
	stickySide = `.verso-grid__cell:last-child:where(.verso-grid:is([data-verso-style="settings"], [data-verso-style="rail"]) *)`
	formSplit  = "@container (width >= 62.5rem)"
)

// Cards of one kind stand in a row of cells, each card in a cell of its own,
// so none is the card after another (sections.css spaces those) and the row
// lays them side by side where the page has room.
func TestGridCardsStandInCells(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "cards", Children: []Widget{
		&Section{Title: "SFP+ module", Chip: "xfi0", Hairline: true},
		&Section{Title: "SFP+ module", Chip: "xfi1", Hairline: true},
	}})
	if !strings.HasPrefix(strings.TrimSpace(got), `<div class="verso-grid" data-verso-style="cards" data-verso-cards`) || !strings.Contains(got, `<div class="verso-grid__track">`) {
		t.Errorf("the row should open the cards' cells:\n%s", got)
	}
	if n := strings.Count(got, `<div class="verso-grid__cell"><section`); n != 2 {
		t.Errorf("%d cards in cells of their own, want 2:\n%s", n, got)
	}
	css := blockCSS(t, "grid")
	declares(t, "grid", `.verso-grid[data-verso-style="cards"]`, "container: cards / inline-size;")
	cards := `.verso-grid__track:where(.verso-grid[data-verso-style="cards"] > *)`
	declares(t, "grid", cards, "grid-template-columns: calc(round(down, 100% - 1px, 8px) + 1px);")
	if rule := cssRuleWithin(css, "@container cards (width >= 40rem)", cards); !strings.Contains(rule, "grid-template-columns: repeat(2,") {
		t.Errorf("grid.css should lay the cards side by side where there is room:\n%s", rule)
	}
}

// A figure stands beside what is said of it: what is said first, taking the
// row, then the figure at its own width (grid.css lays them out and wraps
// them).
func TestGridFigureStandsBesideItsFacts(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "figure", Children: []Widget{
		&Properties{Align: "left", Items: []Property{{Label: "Memory", Value: "7.7 GiB"}}},
		&Ports{Back: `<svg viewBox="0 0 504 136"></svg>`, Items: []PortItem{{Iface: "eth0"}}},
	}})
	row, list, ports := strings.Index(got, `data-verso-style="figure"`), strings.Index(got, "<dl"), strings.Index(got, `class="verso-ports`)
	if row < 0 || list < row || ports < list {
		t.Errorf("the facts should open the row, then the figure:\n%s", got)
	}
	declares(t, "grid", `.verso-grid__track:where(.verso-grid[data-verso-style="figure"] > *)`, "display: flex;", "flex-wrap: wrap;")
	declares(t, "grid", `.verso-grid__cell:not(:last-child):where(.verso-grid[data-verso-style="figure"] *)`, "flex: 1 1 22rem;")
	declares(t, "grid", `.verso-grid__cell:last-child:where(.verso-grid[data-verso-style="figure"] *)`, "flex: 0 1 var(--art-w, auto);")
	// The row's hook is its own: a listing's live figures (a device's Now
	// cell) carry data-verso-figure, and a rule aimed at that would stack
	// them and stretch every row.
	for _, path := range []string{
		filepath.Join("..", "server", "assets", "sections.css"),
		filepath.Join("..", "server", "assets", "blocks", "grid.css"),
	} {
		css, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(css), "[data-verso-figure]") {
			t.Errorf("%s styles the live figures' hook", path)
		}
	}
}

// TestGridColumnsResponsive checks the count→columns mapping the shell owns:
// one column, two-up from a phone, three and four from a larger screen.
func TestGridColumnsResponsive(t *testing.T) {
	r := newRenderer(t)
	for cols := 1; cols <= 4; cols++ {
		got := render(t, r, &Grid{Columns: cols, Children: []Widget{&Text{Markdown: "x"}}})
		if want := `data-verso-columns="` + string(rune('0'+cols)) + `"`; !strings.Contains(got, want) {
			t.Errorf("columns=%d: want %s in:\n%s", cols, want, got)
		}
	}
	css := blockCSS(t, "grid")
	declares(t, "grid", ".verso-grid__track", "grid-template-columns: repeat(2, minmax(0, 1fr));")
	declares(t, "grid", `.verso-grid__track:where(.verso-grid[data-verso-columns="1"] *)`, "grid-template-columns: minmax(0, 1fr);")
	for _, n := range []string{"3", "4"} {
		sel := `.verso-grid__track:where(.verso-grid[data-verso-columns="` + n + `"] *)`
		if rule := cssRuleWithin(css, "@media (width >= 48rem)", sel); !strings.Contains(rule, "grid-template-columns: repeat("+n+", minmax(0, 1fr));") {
			t.Errorf("columns=%s: grid.css should widen to %s columns on a larger screen:\n%s", n, n, rule)
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
		`class="verso-box__part" data-verso-measure="secret"`,
		`id="password" name="password" type="password" autocomplete="new-password" aria-label="New password"`,
		`id="confirm" name="confirm" type="password" autocomplete="new-password" aria-label="Repeat new password"`,
		`data-verso-change-kind="password"`,
		"Repeat new password: The passwords differ.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fused secret missing %q:\n%s", want, got)
		}
	}
	// set in words: the input role's sans, not its verbatim form
	if n := strings.Count(got, `class="verso-box__input">`); n != 2 || strings.Contains(got, "data-verso-verbatim") {
		t.Errorf("a secret is set in words, want 2 parts without verbatim, got %d:\n%s", n, got)
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
	for _, want := range []string{`data-verso-columns="3"`, "Devices", "up"} {
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
		`<div class="verso-grid" data-verso-style="rail" data-verso-columns="2">`,
		`<div data-verso-form-column class="verso-grid__cell">`,
		"the work", "the rail",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rail grid missing %q:\n%s", want, got)
		}
	}
	css := blockCSS(t, "grid")
	declares(t, "grid", ".verso-grid", "container-type: inline-size;")
	// Narrow, one column; a rail is never the even split an ordinary grid draws.
	declares(t, "grid", splitTrack, "grid-template-columns: minmax(0, 1fr);", "gap: calc(var(--spacing) * 10);")
	if rule := cssRuleWithin(css, formSplit, splitTrack); !strings.Contains(rule, "grid-template-columns: var(--verso-form-width, 40rem) minmax(0, 1fr);") {
		t.Errorf("grid.css should split a rail at the form's measure once its container has room:\n%s", rule)
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
		if want := `data-verso-style="` + style + `"`; !strings.Contains(got, want) {
			t.Errorf("%s grid: missing %s:\n%s", style, want, got)
		}
		if !strings.Contains(stickySide, `[data-verso-style="`+style+`"]`) {
			t.Errorf("%s grid: the side column is not among the ones held in view", style)
		}
	}
	declares(t, "grid", splitSide, "align-self: start;")
	rule := cssRuleWithin(blockCSS(t, "grid"), formSplit, stickySide)
	for _, want := range []string{"position: sticky;", "top: calc(var(--spacing) * 22);"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the side column beside the form is missing %q:\n%s", want, rule)
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

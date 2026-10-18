// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestASectionsGlyphLeadsItsTitle: a page's section is a card, and the card's
// header leads with the glyph of what the section is, in Glyph, as the
// Overview's cards do; a section without one shows none.
func TestASectionsGlyphLeadsItsTitle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "SSH", Icon: "terminal"})
	band := strings.Index(got, "data-verso-section-band")
	glyph := strings.Index(got, `<span aria-hidden="true" class="verso-heading__glyph"><svg`)
	title := strings.Index(got, ">SSH</h2>")
	if band < 0 || !(band < glyph && glyph < title) {
		t.Errorf("the glyph does not lead the title in the band:\n%s", got)
	}
	declares(t, "heading", ".verso-heading__glyph", "color: var(--color-glyph);")
	if bare := render(t, r, &Section{Title: "SSH"}); strings.Contains(bare, "verso-heading__glyph") {
		t.Errorf("a section without a glyph shows one:\n%s", bare)
	}
}

// A section is a card when the surface it stands on stands its subjects on
// cards (a page's body, a drawer's) and it is one of that surface's subjects:
// top-level, with a heading or ruled with none. The renderer says so in the
// markup — the card block's classes and data-verso-section="card" — so no
// stylesheet rule has to infer it from where the section stands.

func TestATitledTopLevelSectionIsACard(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Title: "Ports", Children: []Widget{&Text{Markdown: "a"}}})
	for _, want := range []string{
		`<section class="verso-card" data-verso-section="card">`,
		`<div class="verso-card__head verso-heading" data-verso-section-band data-verso-size="card">`,
		`<div class="verso-card__body verso-rhythm">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a titled section on a page is not a card (missing %q):\n%s", want, got)
		}
	}
}

func TestAHeadlessRuledTopLevelSectionIsACard(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Hairline: true, Children: []Widget{&Text{Markdown: "a"}}})
	if !strings.Contains(got, `<section class="verso-card" data-verso-section="card" data-verso-headless>`) {
		t.Errorf("a headless ruled section on a page is not a headless card:\n%s", got)
	}
	if strings.Contains(got, "verso-card__head") {
		t.Errorf("a headless card draws a head:\n%s", got)
	}
}

func TestANestedPartIsNotACard(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Title: "Interface", Children: []Widget{
		&Section{Title: "Addresses", Hairline: true, Children: []Widget{&Text{Markdown: "a"}}},
	}})
	if strings.Count(got, `class="verso-card"`) != 1 {
		t.Errorf("only the outer section is a card:\n%s", got)
	}
	if !strings.Contains(got, `<section class="verso-section" data-verso-section="part"`) {
		t.Errorf("the nested section is not a part:\n%s", got)
	}
}

func TestOnCardsStandsALooseRunInAnUntitledCard(t *testing.T) {
	got := render(t, newRenderer(t), OnCards(&Field{Name: "a", Label: "A"}))
	if !strings.Contains(got, `<section class="verso-card" data-verso-section="card" data-verso-headless`) {
		t.Errorf("a loose run of fields is not stood on an untitled card:\n%s", got)
	}
}

func TestAPlainOrKickerSectionIsNotACard(t *testing.T) {
	r := newRenderer(t)
	for _, s := range []*Section{
		{Children: []Widget{&Text{Markdown: "a"}}},
		{Title: "Group", Kicker: true, Children: []Widget{&Text{Markdown: "a"}}},
		{Hairline: true, Flush: true, Children: []Widget{&Text{Markdown: "a"}}},
	} {
		if got := render(t, r, s); strings.Contains(got, "verso-card") {
			t.Errorf("a section that is no subject of the surface is a card:\n%s", got)
		}
	}
}

// renderPanelBody renders a row drawer's body as the shell's panel does.
func renderPanelBody(t *testing.T, d *RowDrawer) string {
	t.Helper()
	r := newRenderer(t)
	pass := &Renderer{tmpl: r.setFor(""), md: r.md, seq: r.seq}
	v, err := pass.renderPanel(d, "", Flash{})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, part := range v.Body {
		b.WriteString(string(part))
	}
	return b.String()
}

func TestADrawersSubjectsAreCards(t *testing.T) {
	got := renderPanelBody(t, &RowDrawer{Title: "Zone", Children: []Widget{
		&Section{Title: "General", Children: []Widget{&Field{Name: "a", Label: "A"}}},
	}})
	if !strings.Contains(got, `<section class="verso-card" data-verso-section="card">`) {
		t.Errorf("a drawer's titled section is not a card:\n%s", got)
	}
}

func TestAChoosersSectionsAreNotCards(t *testing.T) {
	got := renderPanelBody(t, &RowDrawer{Title: "Add", Size: "choices", Children: []Widget{
		&Section{Title: "Interfaces", Children: []Widget{&Text{Markdown: "a"}}},
	}})
	if strings.Contains(got, "verso-card") {
		t.Errorf("a chooser's sections are cards:\n%s", got)
	}
}

func TestADialogsSectionsAreNotCards(t *testing.T) {
	got := render(t, newRenderer(t), &Modal{Title: "Upload", Trigger: "Upload", Children: []Widget{
		&Section{Title: "File", Children: []Widget{&Text{Markdown: "a"}}},
	}})
	if strings.Contains(got, "verso-card") {
		t.Errorf("a dialog's sections are cards: it is a card already:\n%s", got)
	}
}

func TestASurfaceWithoutCardsDrawsNoCard(t *testing.T) {
	r := newRenderer(t)
	pass := &Renderer{tmpl: r.setFor(""), md: r.md, seq: r.seq, cards: true}
	var b strings.Builder
	if err := pass.withoutCards().render(&b, &Section{Title: "Internet", Children: []Widget{&Text{Markdown: "a"}}}, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "verso-card") {
		t.Errorf("an Overview card's body draws a card inside the card:\n%s", b.String())
	}
}

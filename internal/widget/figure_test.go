// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestAFigureShowsItsPicture: a figure is a picture at its own proportions,
// named for a screen reader, its size declared so the page does not move when
// it arrives.
func TestAFigureShowsItsPicture(t *testing.T) {
	got := render(t, newRenderer(t), &Figure{Src: "/assets/boards/0123456789ab.png", Alt: "Mono Gateway Development Kit", Width: 800, Height: 592})
	want := `<figure class="verso-figure" style="--verso-figure-ratio: 0.7400"><img class="verso-figure__image" src="/assets/boards/0123456789ab.png" alt="Mono Gateway Development Kit" width="800" height="592" decoding="async"></figure>`
	if !strings.Contains(got, want) {
		t.Errorf("the figure is not its picture, want %s in:\n%s", want, got)
	}
	// Beside what it shows it takes about a third of the row, never under
	// 256px, so a phone's card, where it stands under the facts, still shows it.
	declares(t, "figure", ".verso-figure", "container-type: inline-size;", "width: 100%;", "min-width: calc(var(--spacing) * 64);", "max-width: 100%;", "margin: 0;")
	declares(t, "grid", ".verso-grid__cell:has(> .verso-figure)", "--art-w: max(30%, calc(var(--spacing) * 64));")
	// However wide the figure stands, the picture's height lands on the 8px
	// grid: its proportions rounded down to a whole line, the picture held
	// within it, so a photograph taller than the facts beside it keeps the
	// card on the grid.
	declares(t, "figure", ".verso-figure__image",
		"display: block;",
		"width: 100%;",
		"height: round(down, 100cqi * var(--verso-figure-ratio), var(--spacing) * 2);",
		"object-fit: contain;")
}

// TestAFigureWithoutAPictureSaysWhy: where there is no picture, the figure is
// a dashed box holding the one sentence that says why, so the card keeps
// its shape and the reader learns what is missing.
func TestAFigureWithoutAPictureSaysWhy(t *testing.T) {
	got := render(t, newRenderer(t), &Figure{Note: "No profile for this board"})
	want := `<figure class="verso-figure" data-verso-empty><figcaption class="verso-figure__note">No profile for this board</figcaption></figure>`
	if !strings.Contains(got, want) {
		t.Errorf("the empty figure does not say why, want %s in:\n%s", want, got)
	}
	if strings.Contains(got, "<img") {
		t.Errorf("an empty figure draws a picture:\n%s", got)
	}
	// The box fills its room: the figure's cell across, and down to the foot
	// of what stands beside it (rows on the grid, so the box is too), never
	// under five lines, room for its words and link.
	declares(t, "figure", ".verso-figure[data-verso-empty]",
		"height: 100%;",
		"min-height: calc(var(--spacing) * 40);",
		"place-content: center;",
		"justify-items: center;",
		// the place-marking frame every empty place wears (the slot's): a
		// dashed Strong Hairline on Quiet Sand
		"border: 1px dashed var(--color-rule-strong);",
		"background-color: var(--color-quiet);")

	// Where the reader can do something about it, the link to how stands
	// under the sentence: the link block, leaving the router in a new tab.
	linked := render(t, newRenderer(t), &Figure{Note: "No profile for this board", Link: &Link{Label: "How to add a profile", Href: "https://example.org/contributing#profiles", NewTab: true}})
	if !strings.Contains(linked, `No profile for this board</figcaption><a href="https://example.org/contributing#profiles" class="verso-link" target="_blank" rel="noopener noreferrer">`) {
		t.Errorf("the empty figure does not link to how to fill it:\n%s", linked)
	}
}

// TestAFiguresLinkIsItsChild: the walk reaches a figure's link, so its
// words are localized as every link's are.
func TestAFiguresLinkIsItsChild(t *testing.T) {
	link := &Link{Label: "How to add a profile", Href: "/x"}
	if kids := (&Figure{Link: link}).children(); len(kids) != 1 || kids[0] != link {
		t.Errorf("children = %v, want the link", kids)
	}
	declares(t, "figure", ".verso-figure__note", "color: var(--color-meta);", "@apply type-body;", "text-align: center;")
}

// TestAFiguresNoteIsProse: the sentence an empty figure says is the reader's
// language; the picture's name is a name, left as it is.
func TestAFiguresNoteIsProse(t *testing.T) {
	f := &Figure{Note: "No profile for this board", Alt: "Mono Gateway Development Kit"}
	translateSchema(f, func(s string) string { return "«" + s + "»" })
	if f.Note != "«No profile for this board»" {
		t.Errorf("note = %q, want it translated", f.Note)
	}
	if f.Alt != "Mono Gateway Development Kit" {
		t.Errorf("alt = %q, want the board's name as it is", f.Alt)
	}
}

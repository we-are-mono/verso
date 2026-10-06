// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAButtonSaysWhatItDoesWhilePressed: a button whose press starts work the
// page waits on (an update check) names that work while it runs, from the
// label it carries for the moment of the press.
func TestAButtonSaysWhatItDoesWhilePressed(t *testing.T) {
	got := render(t, newRenderer(t), &Button{Label: "Check again", Busy: "Checking…", Style: "act", Name: "action", Value: "check"})
	if !strings.Contains(got, `data-busy-label="Checking…"`) {
		t.Errorf("the button does not carry what it says while pressed:\n%s", got)
	}
	if plain := render(t, newRenderer(t), &Button{Label: "Save"}); strings.Contains(plain, "data-busy-label") {
		t.Errorf("a button with nothing to say while pressed carries a busy label:\n%s", plain)
	}
}

// TestACalloutCarriesTheActsThatResolveIt: a notice that something is missing
// carries the act that resolves it, inside the band: a row of buttons a cell
// under the words, centred in two cells, in the band's own ink.
func TestACalloutCarriesTheActsThatResolveIt(t *testing.T) {
	for _, compact := range []bool{true, false} {
		got := render(t, newRenderer(t), &Callout{Variant: "info", Compact: compact, Title: "Nothing is blocked", Body: "Installing adds a blocklist.",
			Acts: []Link{{Style: "secondary", Label: "Install adblock", Icon: "download", Href: "/system/packages/package?name=adblock", Panel: true}}})
		row := strings.Index(got, `<div data-verso-callout-acts class="mt-5 flex flex-wrap items-center gap-x-3 gap-y-1.5 py-0.75">`)
		words := strings.Index(got, "Installing adds a blocklist.")
		if row < 0 || words < 0 || row < words {
			t.Errorf("compact=%v: the acts row stands inside the band, under its words:\n%s", compact, got)
		}
		if !strings.Contains(got[row:], ">Install adblock</a>") || !strings.Contains(got[row:], `@click.prevent="showPanel"`) {
			t.Errorf("compact=%v: the act keeps its button and its panel:\n%s", compact, got)
		}
	}
	// In the band's ink: the stylesheet colours an act inside a band from it.
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "[data-verso-callout-acts] a {") {
		t.Error("an act inside a band is not coloured from it")
	}
}

// TestACalloutCarriesTheMachinesWords: what a tool reported rides inside the
// band, under a hairline of the band's tone, verbatim in mono, with a copy
// control beside it.
func TestACalloutCarriesTheMachinesWords(t *testing.T) {
	got := render(t, newRenderer(t), &Callout{Variant: "warning", Title: "The check could not finish", Body: "The update tool stopped.", Verbatim: "owut: no route to host"})
	for _, want := range []string{"border-t ", "border-marigold-line", "font-mono", "owut: no route to host", `x-data="copy"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the band does not carry the machine's words (%q):\n%s", want, got)
		}
	}
	if plain := render(t, newRenderer(t), &Callout{Variant: "warning", Body: "Nothing verbatim."}); strings.Contains(plain, "font-mono") {
		t.Errorf("a band with no machine words draws a mono line:\n%s", plain)
	}
}

// TestAFormsActCanBeTheDangerAct: a form whose act destroys something (erasing
// the router) wears the danger act rather than the primary.
func TestAFormsActCanBeTheDangerAct(t *testing.T) {
	got := render(t, newRenderer(t), &Form{Action: "/system/maintenance/factory-reset", Submit: "Erase and start over", Tone: ToneDanger})
	if !strings.Contains(got, "bg-crimson") || strings.Contains(got, "bg-denim") {
		t.Errorf("the destroying act is not the danger act:\n%s", got)
	}
}

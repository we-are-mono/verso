// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
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

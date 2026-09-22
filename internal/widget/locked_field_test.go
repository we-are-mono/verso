// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestLockedFieldShowsAValueThatCannotChange: a value fixed once written — a
// network's mode, a radio's band — keeps its row in the form, with its label
// and the option it wrote, but the control is a quiet box with a padlock and
// the reason on it, and it posts nothing.
func TestLockedFieldShowsAValueThatCannotChange(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "mode", Label: "Mode", Key: "mode", Style: "locked", Value: "Access point",
		Help: "The mode is fixed once written — remove the network and add a new one to change it.",
	})
	for _, want := range []string{
		`<label for="mode"`,              // the row's label still names the control
		`<output id="mode"`,              // a labelable element that submits nothing
		"border-rule bg-quiet text-body", // the canvas's locked box
		">Access point<",
		`title="The mode is fixed once written`,
		lucideIcons["lock"],
		">mode<", // the key chip still cites the option
	} {
		if !strings.Contains(got, want) {
			t.Errorf("locked field missing %q:\n%s", want, got)
		}
	}
	for _, never := range []string{"<input", "<select", `name="mode"`} {
		if strings.Contains(got, never) {
			t.Errorf("a locked field posts nothing, found %q:\n%s", never, got)
		}
	}
}

// TestLockedFieldValueIsWords: what the box reads is the value said plainly
// ("Access point", "5 GHz"), so it translates like the label beside it.
func TestLockedFieldValueIsWords(t *testing.T) {
	f := &Field{Name: "mode", Label: "Mode", Style: "locked", Value: "Access point"}
	translateSchema(f, func(s string) string {
		if s == "Access point" {
			return "Dostopna točka"
		}
		return s
	})
	if f.Value != "Dostopna točka" {
		t.Errorf("locked value = %q, want it translated", f.Value)
	}
	plain := &Field{Name: "ssid", Label: "Network name", Value: "Access point"}
	translateSchema(plain, func(string) string { return "changed" })
	if plain.Value != "Access point" {
		t.Error("an editable field's value is data and must stay verbatim")
	}
}

// TestWifiCountsTakeTheNumberMeasure: a network's client ceiling is a count, so
// its box is the 96px number measure, not the full width of a name.
func TestWifiCountsTakeTheNumberMeasure(t *testing.T) {
	if got := (&Field{Name: "maxassoc", Key: "maxassoc"}).Measure(); got != "number" {
		t.Errorf("maxassoc measure = %q, want number", got)
	}
}

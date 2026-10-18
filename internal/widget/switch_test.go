// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestDecodeAndRenderSwitch(t *testing.T) {
	w, err := Decode([]byte(`{"type":"switch","name":"enabled","label":"Enabled","help":"Applies immediately.","on":true}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got := render(t, newRenderer(t), w)
	for _, want := range []string{
		`type="checkbox"`, `data-verso-switch`, `value="1"`, `name="enabled"`, " checked",
		"Enabled", `class="verso-choice__input"`,
		// A switch row is a field row like any other: its name is the
		// question and its box the answer.
		`<div class="verso-field" data-verso-change-field`,
		`<label for="enabled" id="enabled-label" class="verso-field__name">`,
		`<div data-verso-control class="verso-field__answer">`,
		// The input is screen-reader-only, so the visible box has to be inside
		// a label: without one, pressing the control does nothing at all.
		`<label class="verso-choice__row"><span class="verso-choice">`,
		// And its help is its description under its name, as a field's is,
		// and the box is read with it.
		`<p id="enabled-desc" data-verso-field-desc class="verso-field__desc"><span>Applies immediately.</span></p>`,
		`aria-describedby="enabled-desc"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("switch missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "data-verso-tip") || strings.Contains(got, "enabled-tip") {
		t.Errorf("a switch's name raises no tooltip:\n%s", got)
	}
	// The row keeps the form's measure, and its name the field-label role.
	declares(t, "field", ".verso-field", "max-width: var(--verso-form-width, 40rem);")
	declares(t, "field", ".verso-field__name", "color: var(--color-ink);", "@apply type-field-label;")
	// DOM and visual order agree: the question, then the answer.
	if label, control := strings.Index(got, ">Enabled"), strings.Index(got, "data-verso-switch"); label < 0 || control < 0 || label > control {
		t.Errorf("the switch's name comes before its box in DOM order: %s", got)
	}
	inline := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", Style: "inline", On: true})
	for _, want := range []string{
		`<label class="verso-choice__row" data-verso-kind="inline"`,
		`<span aria-hidden="true" class="verso-choice__dot"></span><span class="verso-choice">`,
		`<span class="verso-choice__word"><span class="verso-choice__on">Enabled</span><span class="verso-choice__off">Disabled</span></span></label>`,
	} {
		if !strings.Contains(inline, want) {
			t.Errorf("inline switch missing %q: %s", want, inline)
		}
	}
	// A heading's switch: its inert dot, the box 12px after it a pixel down,
	// and the state's own words in ink in the field label's role.
	choice := blockCSS(t, "choice")
	declares(t, "choice", ".verso-choice__dot", "width: var(--spacing)", "border-radius: calc(infinity * 1px)", "background-color: var(--color-inert)")
	declares(t, "choice", `.verso-choice:where(.verso-choice__row[data-verso-kind="inline"] *)`, "margin-left: calc(var(--spacing) * 3)", "translate: 0 1px")
	declares(t, "choice", `.verso-choice__word:where(.verso-choice__row[data-verso-kind="inline"] *)`, "color: var(--color-ink)", "@apply type-field-label")
	if !strings.Contains(choice, ".verso-choice__on:where(.verso-choice__row:not(:has(.verso-choice__input:checked)) *),\n.verso-choice__off:where(.verso-choice__row:has(.verso-choice__input:checked) *) {\n  display: none;") {
		t.Errorf("the inline switch does not show the words for its state:\n%s", choice)
	}
}

// TestAFormSwitchSaysOnOrOff: in a form row a switch is its box and the word
// for its state beside it, inside one label, so the name, the box and the word
// all flip it (form-fields). The stylesheet shows the word the box is in; the
// checkbox already says its state to a screen reader, so the words are hidden
// from it.
func TestAFormSwitchSaysOnOrOff(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Switch{Name: "wmm", Label: "WMM", On: true})
	word := `<span aria-hidden="true" class="verso-choice__word"><span class="verso-choice__on">On</span><span class="verso-choice__off">Off</span></span></label>`
	if !strings.Contains(got, word) {
		t.Errorf("a form switch does not say its state beside its box:\n%s", got)
	}
	if box := strings.Index(got, "data-verso-switch"); box < 0 || box > strings.Index(got, "verso-choice__word") {
		t.Errorf("the word follows the box:\n%s", got)
	}
	declares(t, "choice", ".verso-choice__row", "gap: calc(var(--spacing) * 2)")
	declares(t, "choice", ".verso-choice__word", "color: var(--color-body)", "@apply type-body")
}

// TestSwitchControlIsACheckbox: the on/off control is a checkbox, never a
// toggle — nothing in the app flips live, every state is staged for the
// commit, and a knob would promise otherwise. The box uses the shared choice
// colour and retains its check mark and separate keyboard focus outline.
func TestSwitchControlIsACheckbox(t *testing.T) {
	got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", On: true})
	for _, want := range []string{
		`<span class="verso-choice"><input type="checkbox" data-verso-switch`,
		`class="verso-choice__input" value="1" checked><span class="verso-choice__face"></span><span class="verso-choice__check">`,
		lucideIcons["check"],
	} {
		if !strings.Contains(got, want) {
			t.Errorf("checkbox missing %q:\n%s", want, got)
		}
	}
	declares(t, "choice", ".verso-choice", "width: calc(var(--spacing) * 4)", "height: calc(var(--spacing) * 4)")
	declares(t, "choice", ".verso-choice__face", "border: 1px solid var(--color-choice-border)",
		"border-radius: var(--radius-xs)", "background-color: var(--color-canvas)", "box-shadow: var(--shadow-field)")
	declares(t, "choice", ".verso-choice__face:is(.verso-choice__input:checked + *)",
		"border-color: var(--color-choice)", "background-color: var(--color-choice)")
	declares(t, "choice", ".verso-choice__face:is(.verso-choice__input:focus-visible + *)",
		"outline: 2px solid var(--color-denim-deep)", "outline-offset: 2px")
	declares(t, "choice", ".verso-choice__check", "color: var(--color-on-fill)", "opacity: 0")
	declares(t, "choice", ".verso-choice__check:is(.verso-choice__input:checked ~ *)", "opacity: 1")
	declares(t, "choice", ".verso-choice__check svg", "stroke-width: 3")
	for _, absent := range []string{"rounded-full", "translate-x-5", "hover:border", "hover:bg"} {
		if strings.Contains(got, absent) {
			t.Errorf("a checkbox keeps its square shape and shared choice colour; found %q:\n%s", absent, got)
		}
	}
	face := cssRule(blockCSS(t, "choice"), ".verso-choice__face")
	if strings.Contains(face, "9999px") || strings.Contains(face, "infinity") || strings.Contains(face, "--color-body") {
		t.Errorf("a checkbox keeps its square shape and shared choice colour:\n%s", face)
	}
}

// TestSwitchDeclaresChangeHooks: every switch style wraps itself in the
// staged-changes hooks, so a page-form submission that flips only a switch
// still counts as a change — the same contract field, list, conditional, and
// settings rows declare.
func TestSwitchDeclaresChangeHooks(t *testing.T) {
	for _, style := range []string{"", "checkbox", "inline"} {
		got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", Style: style, On: true})
		for _, want := range []string{
			`data-verso-change-field`, `data-verso-change-name="enabled"`,
			`data-verso-change-label="Enabled"`, `data-verso-change-kind="toggle"`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("style %q switch missing %q:\n%s", style, want, got)
			}
		}
	}
	nameless := render(t, newRenderer(t), &Switch{Label: "Enabled"})
	if strings.Contains(nameless, "data-verso-change-field") {
		t.Errorf("a nameless switch posts nothing and must declare no change hook:\n%s", nameless)
	}
}

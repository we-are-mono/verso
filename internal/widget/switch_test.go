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
		"Enabled", "peer-checked:bg-choice",
		// A switch row is a field row like any other: its name is the
		// question and its box the answer.
		`class="verso-field-row max-w-form"`,
		`<label for="enabled" id="enabled-label" class="text-sm font-medium text-ink`,
		"verso-field-control verso-field-answer",
		// The input is screen-reader-only, so the visible box has to be inside
		// a label: without one, pressing the control does nothing at all.
		`<label class="flex w-fit cursor-pointer items-center"><span class="relative inline-flex size-4`,
		// And its help is its description under its name, as a field's is,
		// and the box is read with it.
		`<p id="enabled-desc" data-verso-field-desc class="text-sm leading-6 text-pretty text-body">Applies immediately.</p>`,
		`aria-describedby="enabled-desc"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("switch missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "data-verso-tip") || strings.Contains(got, "enabled-tip") {
		t.Errorf("a switch's name raises no tooltip:\n%s", got)
	}
	// DOM and visual order agree: the question, then the answer.
	if label, control := strings.Index(got, ">Enabled"), strings.Index(got, "data-verso-switch"); label < 0 || control < 0 || label > control {
		t.Errorf("the switch's name comes before its box in DOM order: %s", got)
	}
	inline := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", OffLabel: "Disabled", Style: "inline", On: true})
	for _, want := range []string{"verso-inline-switch inline-flex cursor-pointer", "size-1 rounded-full bg-inert", "ml-3 inline-flex translate-y-px", "verso-inline-switch-on", "Enabled", "verso-inline-switch-off", "Disabled", "text-ink"} {
		if !strings.Contains(inline, want) {
			t.Errorf("inline switch missing %q: %s", want, inline)
		}
	}
}

// TestAFormSwitchSaysOnOrOff: in a form row a switch is its box and the word
// for its state beside it, inside one label, so the name, the box and the word
// all flip it (form-fields). The stylesheet shows the word the box is in; the
// checkbox already says its state to a screen reader, so the words are hidden
// from it. A table's toggle cell stays a bare box.
func TestAFormSwitchSaysOnOrOff(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Switch{Name: "wmm", Label: "WMM", On: true})
	word := `<span aria-hidden="true" class="verso-switch-word ml-2 text-sm leading-6 text-body"><span class="verso-switch-on">On</span><span class="verso-switch-off">Off</span></span></label>`
	if !strings.Contains(got, word) {
		t.Errorf("a form switch does not say its state beside its box:\n%s", got)
	}
	if box := strings.Index(got, "data-verso-switch"); box < 0 || box > strings.Index(got, "verso-switch-word") {
		t.Errorf("the word follows the box:\n%s", got)
	}
	cell := render(t, r, &Table{
		Columns: []TableColumn{{Kind: "toggle"}, {Label: "Name", Kind: "name"}},
		Rows:    []TableRow{{ID: "r1", Cells: []TableCell{{Name: "r1_enabled", On: true}, {Text: "Allow-SSH"}}}},
	})
	if !strings.Contains(cell, "data-verso-switch") || strings.Contains(cell, "verso-switch-word") {
		t.Errorf("a table's toggle cell is a bare box:\n%s", cell)
	}
}

// TestSwitchControlIsACheckbox: the on/off control is a checkbox, never a
// toggle — nothing in the app flips live, every state is staged for the
// commit, and a knob would promise otherwise. The box uses the shared choice
// colour and retains its check mark and separate keyboard focus outline.
func TestSwitchControlIsACheckbox(t *testing.T) {
	got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", On: true})
	for _, want := range []string{
		`<span class="relative inline-flex size-4 shrink-0 cursor-pointer items-center justify-center">`,
		`class="absolute inset-0 rounded-xs border border-choice-border bg-canvas shadow-(--shadow-field) transition-colors peer-checked:border-choice peer-checked:bg-choice peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-denim-deep peer-disabled:border-inert peer-disabled:shadow-none peer-disabled:peer-checked:bg-inert"`,
		`<span class="relative text-on-fill opacity-0 peer-checked:opacity-100">`, lucideIcons["check"], "[stroke-width:3]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("checkbox missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"rounded-full", "translate-x-5", "peer-checked:bg-body", "hover:border", "hover:bg"} {
		if strings.Contains(got, absent) {
			t.Errorf("a checkbox keeps its square shape and shared choice colour; found %q:\n%s", absent, got)
		}
	}
}

// TestSwitchDeclaresChangeHooks: every switch style wraps itself in the
// staged-changes hooks, so a page-form submission that flips only a switch
// still counts as a change — the same contract field, list, conditional, and
// settings rows declare.
func TestSwitchDeclaresChangeHooks(t *testing.T) {
	for _, style := range []string{"", "checkbox", "inline"} {
		got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", OffLabel: "Disabled", Style: style, On: true})
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

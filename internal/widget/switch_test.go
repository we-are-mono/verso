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
		"Enabled", "peer-checked:bg-body",
		// A state to flip is one setting, so it is the form's one row: the same
		// measure, the same label column, the same fixed control column a value
		// to type gets. A settings page that drew them differently read as two
		// forms stacked.
		"flex max-w-[40rem] flex-col gap-2 py-3 sm:flex-row sm:items-start sm:gap-8",
		`<label for="enabled" id="enabled-label" class="text-sm font-semibold text-ink`,
		"flex w-full min-w-0 flex-col justify-center gap-1.5 sm:w-64 sm:min-h-9 sm:flex-none",
		// The input is screen-reader-only, so the visible box has to be inside
		// a label: without one, pressing the control does nothing at all.
		`<label class="flex w-fit cursor-pointer items-center"><span class="relative inline-flex size-4.5`,
		// And its explanation is raised onto the label, as a field's is, rather
		// than taking a paragraph indented under the control.
		"group/tip", `aria-describedby="enabled-tip"`, "Applies immediately.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("switch missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "pl-12 text-sm leading-snug text-body") {
		t.Errorf("help must not take a paragraph under the control:\n%s", got)
	}
	// The label leads the row and the control ends it — the reading order every
	// other row has, not the control-first order a bare checkbox would take.
	if label, control := strings.Index(got, ">Enabled"), strings.Index(got, "data-verso-switch"); label < 0 || control < 0 || label > control {
		t.Errorf("the label leads the row, the switch ends it: %s", got)
	}
	inline := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", OffLabel: "Disabled", Style: "inline", On: true})
	for _, want := range []string{"verso-inline-switch inline-flex cursor-pointer", "size-1 rounded-full bg-inert", "ml-3 inline-flex translate-y-px", "verso-inline-switch-on", "Enabled", "verso-inline-switch-off", "Disabled", "text-ink"} {
		if !strings.Contains(inline, want) {
			t.Errorf("inline switch missing %q: %s", want, inline)
		}
	}
}

// TestSwitchControlIsACheckbox: the on/off control is a checkbox, never a
// toggle — nothing in the app flips live, every state is staged for the
// commit, and a knob would promise otherwise. The box wears the input's own
// dress with no hover state, and checks in the body ink a selected segment
// takes, never the action colour.
func TestSwitchControlIsACheckbox(t *testing.T) {
	got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", On: true})
	for _, want := range []string{
		`<span class="relative inline-flex size-4.5 shrink-0 cursor-pointer items-center justify-center">`,
		`class="absolute inset-0 rounded-xs border border-rule-strong bg-white shadow-[inset_0_1px_2px_rgba(27,25,23,.06)] transition-colors peer-checked:border-body peer-checked:bg-body peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-denim"`,
		`<span class="relative text-white opacity-0 peer-checked:opacity-100">`, lucideIcons["check"], "[stroke-width:3]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("checkbox missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"rounded-full", "translate-x-5", "peer-checked:bg-denim", "hover:border", "hover:bg"} {
		if strings.Contains(got, absent) {
			t.Errorf("a checkbox has no track, no knob, no action colour and no hover state; found %q:\n%s", absent, got)
		}
	}
}

// TestSwitchDeclaresChangeHooks: every switch style wraps itself in the
// staged-changes hooks, so a page-form submission that flips only a switch
// still counts as a change — the same contract field, list, conditional, and
// settings rows declare.
func TestSwitchDeclaresChangeHooks(t *testing.T) {
	for _, style := range []string{"", "inline", "hero"} {
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

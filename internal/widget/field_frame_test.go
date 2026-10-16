// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"regexp"
	"strings"
	"testing"
)

// TestEveryLabelledRowIsOneFrame: a value to type, a list to edit, a state to
// flip, a settings row and a conditional's gate are one kind of row. Each names
// the setting, wears the option it writes as a key chip and the stage's mark
// while its change waits — drawn by the one label, never by hand.
func TestEveryLabelledRowIsOneFrame(t *testing.T) {
	r := newRenderer(t)
	key := `<span class="verso-chip my-0.75 inline-flex items-center align-top gap-1.5 rounded-xs border px-1.5 leading-4 whitespace-nowrap text-sm font-medium font-mono border-rule bg-quiet text-meta">opt</span>`
	for name, w := range map[string]Widget{
		"field":    &Field{Name: "opt", Label: "Option", Key: "opt", Staged: true},
		"code":     &Field{Name: "opt", Label: "Option", Key: "opt", Style: "code", Staged: true},
		"locked":   &Field{Name: "opt", Label: "Option", Key: "opt", Style: "locked", Value: "ap", Staged: true},
		"list":     &List{Name: "opt", Label: "Option", Key: "opt", Staged: true},
		"switch":   &Switch{Name: "opt", Label: "Option", Key: "opt", Staged: true},
		"checkbox": &Switch{Name: "opt", Label: "Option", Key: "opt", Style: "checkbox", Staged: true},
		"value":    &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Name: "opt", Value: "1", Staged: true}}},
		"inline":   &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Name: "opt", Value: "1", Inline: true, Staged: true}}},
		"toggle":   &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Toggle: &SettingsToggle{Name: "opt"}, Staged: true}}},
		"gate":     &Conditional{Name: "opt", Label: "Option", Key: "opt", Staged: true},
	} {
		got := render(t, r, w)
		for _, want := range []string{`class="verso-field-label`, key, "data-verso-staged-row"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the row's label is missing %q:\n%s", name, want, got)
			}
		}
		if n := strings.Count(got, "verso-field-row"); n != 1 {
			t.Errorf("%s: one setting is one row, got %d:\n%s", name, n, got)
		}
	}
}

// answerIsTheControl finds the answer column's element, the one the stage and
// the reshape wait read as the row's control.
var answerIsTheControl = regexp.MustCompile(`<div data-verso-control[^>]*class="[^"]*verso-field-answer`)

// TestEveryRowIsAQuestionAndAnAnswer: a field row is one shape whatever it
// holds (form-fields): its question (the name, its chip, the stage's mark) on
// the left of the ledger and its answer, the control, in the answer column. A
// switch is an answer like any other, so no row puts its box before its name,
// and a removable row keeps its glyph in a lane after the answer.
func TestEveryRowIsAQuestionAndAnAnswer(t *testing.T) {
	r := newRenderer(t)
	for name, w := range map[string]Widget{
		"field":     &Field{Name: "opt", Label: "Option", Key: "opt", Staged: true},
		"code":      &Field{Name: "opt", Label: "Option", Key: "opt", Style: "code"},
		"locked":    &Field{Name: "opt", Label: "Option", Key: "opt", Style: "locked", Value: "ap"},
		"list":      &List{Name: "opt", Label: "Option", Key: "opt"},
		"switch":    &Switch{Name: "opt", Label: "Option", Key: "opt"},
		"checkbox":  &Switch{Name: "opt", Label: "Option", Key: "opt", Style: "checkbox"},
		"value":     &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Name: "opt", Value: "1"}}},
		"inline":    &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Name: "opt", Value: "1", Inline: true}}},
		"toggle":    &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Toggle: &SettingsToggle{Name: "opt"}}}},
		"gate":      &Conditional{Name: "opt", Label: "Option", Key: "opt"},
		"removable": &Field{Name: "opt", Label: "Option", Key: "opt", Remove: "yes"},
	} {
		got := render(t, r, w)
		ledger := strings.Index(got, `class="verso-field-ledger`)
		question := strings.Index(got, `class="verso-field-question`)
		label := strings.Index(got, `class="verso-field-label`)
		answer := strings.Index(got, `verso-field-answer`)
		if ledger < 0 || question < ledger || label < question || answer < label {
			t.Errorf("%s: the row is not its question, then its answer, in one ledger:\n%s", name, got)
		}
		if !answerIsTheControl.MatchString(got) {
			t.Errorf("%s: the answer is not the row's control:\n%s", name, got)
		}
		for _, gone := range []string{"verso-field-toggle", "verso-field-with-remove"} {
			if strings.Contains(got, gone) {
				t.Errorf("%s: the row still draws %s:\n%s", name, gone, got)
			}
		}
	}
	got := render(t, r, &Field{Name: "opt", Label: "Option", Remove: "yes"})
	if remove := strings.Index(got, "data-verso-remove-field"); remove < strings.Index(got, "verso-field-answer") {
		t.Errorf("a removable row's glyph stands in the lane after its answer:\n%s", got)
	}
}

// TestAFieldsRefusalIsTheBand: wherever a value is refused, the refusal is the
// crimson band under the box — named by the box, so a screen reader reads it —
// including the code editor, which drew a bare red line of its own.
func TestAFieldsRefusalIsTheBand(t *testing.T) {
	r := newRenderer(t)
	band := `<span id="script-error" data-verso-error class="flex items-start gap-2 rounded-xs bg-crimson-soft px-3 py-2 text-sm leading-6 text-crimson-deep">`
	for name, w := range map[string]Widget{
		"text": &Field{Name: "script", Label: "Script", Error: "Not a script."},
		"code": &Field{Name: "script", Label: "Script", Style: "code", Error: "Not a script."},
	} {
		got := render(t, r, w)
		if !strings.Contains(got, band) || !strings.Contains(got, `aria-describedby="script-error"`) {
			t.Errorf("%s: the refusal is the band the box points at:\n%s", name, got)
		}
	}
	// An in-place value checks itself: its slot is the same band, empty and
	// hidden until the check has something to say.
	got := render(t, r, &Settings{Items: []SettingsItem{{Title: "Hostname", Name: "hostname", Value: "gw", Inline: true}}})
	slot := `<span id="hostname-error" data-verso-inline-error hidden class="flex items-start gap-2 rounded-xs bg-crimson-soft px-3 py-2 text-sm leading-6 text-crimson-deep"></span>`
	if !strings.Contains(got, slot) {
		t.Errorf("an in-place value's slot is the refusal band:\n%s", got)
	}
}

// TestSettingsRowsTakeTheStage: a settings row that edits an option is a
// control like any other, so the shell marks it while its change waits. A row
// that only reads has nothing waiting, whatever its chip says.
func TestSettingsRowsTakeTheStage(t *testing.T) {
	block := &Settings{
		Items: []SettingsItem{
			{Title: "Hand out addresses", Code: "ignore", Toggle: &SettingsToggle{Name: "lan.ignore"}},
			{Title: "Lease length", Code: "leasetime", Name: "lan.leasetime", Value: "12h"},
			{Title: "Address range", Code: "start", Value: "100"},
		},
		Seam: &SettingsSeam{Summary: "more", Items: []SettingsItem{
			{Title: "Authoritative", Code: "force", Toggle: &SettingsToggle{Name: "lan.force"}},
		}},
	}
	staged := map[string]bool{"dhcp.lan.ignore": true, "dhcp.lan.leasetime": true, "dhcp.lan.start": true, "dhcp.lan.force": true}
	MarkStaged(&Form{Target: "dhcp.lan", Fields: []Widget{block}}, func(address string) bool { return staged[address] })
	if !block.Items[0].Staged || !block.Items[1].Staged || !block.Seam.Items[0].Staged {
		t.Errorf("an editing row whose option waits is marked: %+v %+v", block.Items, block.Seam.Items)
	}
	if block.Items[2].Staged {
		t.Error("a row that only reads is never marked")
	}
}

// TestAConditionalGateTakesTheStage: the gate is the setting the rest depend
// on, and its option waits on the stage the way a switch's does.
func TestAConditionalGateTakesTheStage(t *testing.T) {
	gate := &Conditional{Name: "enabled", Label: "Enabled", Key: "enabled"}
	MarkStaged(&Form{Target: "wireguard.wg0", Fields: []Widget{gate}}, func(address string) bool {
		return address == "wireguard.wg0.enabled"
	})
	if !gate.Staged {
		t.Error("a gate whose option waits is marked")
	}
}

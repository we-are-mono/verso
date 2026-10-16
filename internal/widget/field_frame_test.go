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

// TestNoRowsNameRaisesATooltip: what a setting does is written under its
// name, so no row's name raises a tooltip, whatever widget draws the row and
// whatever it was told (form-fields). A condition's name is not a field's and
// keeps its own.
func TestNoRowsNameRaisesATooltip(t *testing.T) {
	r := newRenderer(t)
	help, tip := "What it does.", "What else to know."
	for name, w := range map[string]Widget{
		"field":    &Field{Name: "opt", Label: "Option", Key: "opt", Help: help, Tip: tip},
		"radios":   &Field{Name: "opt", Label: "Option", Key: "opt", Kind: "select", Options: choiceOptions(2), Help: help, Tip: tip},
		"list":     &List{Name: "opt", Label: "Option", Key: "opt", Help: help, Tip: tip},
		"switch":   &Switch{Name: "opt", Label: "Option", Key: "opt", Help: help, Tip: tip},
		"gate":     &Conditional{Name: "opt", Label: "Option", Key: "opt", Help: help},
		"settings": &Settings{Items: []SettingsItem{{Title: "Option", Code: "opt", Name: "opt", Value: "1", Desc: help}}},
		"fused":    synflood(),
		"group":    includes(),
	} {
		got := render(t, r, w)
		for _, gone := range []string{"data-verso-tip", `role="tooltip"`, "cursor-help", "-tip\""} {
			if strings.Contains(got, gone) {
				t.Errorf("%s: a row's name raises a tooltip (%s):\n%s", name, gone, got)
			}
		}
	}
}

// TestAWideAnswerStandsUnderItsQuestion: an answer that cannot stand in the
// 272px answer column stands under its question at the row's full width
// (form-fields): a code or text area, an endpoint, a weekday strip, every list,
// and a fused or joined value whose parts together are wider than the column.
// One that fits keeps the column.
func TestAWideAnswerStandsUnderItsQuestion(t *testing.T) {
	r := newRenderer(t)
	days := []Option{{Value: "mon", Label: "Mon"}, {Value: "tue", Label: "Tue"}}
	periods := []Option{{Value: "second", Label: "second"}, {Value: "minute", Label: "minute"}, {Value: "hour", Label: "hour"}, {Value: "day", Label: "day"}}
	for name, c := range map[string]struct {
		w    Widget
		wide bool
	}{
		"code":     {&Field{Name: "contents", Label: "Contents", Style: "code"}, true},
		"textarea": {&Field{Name: "note", Label: "Note", Kind: "textarea"}, true},
		"endpoint": {&Field{Name: "resolver_url", Label: "Resolver", Key: "resolver_url"}, true},
		"weekdays": {&Field{Name: "days", Label: "Days", Kind: "checks", Style: "segmented", Options: days}, true},
		"list":     {&List{Name: "proto", Label: "Protocol"}, true},
		"text":     {&Field{Name: "name", Label: "Name"}, false},
		"switch":   {&Switch{Name: "on", Label: "On"}, false},
		"rate": {&Grid{Style: "form", Label: "Connection rate", Join: "per", Children: []Widget{
			&Field{Name: "synflood_rate", Label: "Connections", Key: "synflood_rate"},
			&Field{Name: "synflood_rate_unit", Label: "Per", Kind: "select", Options: periods},
		}}, false},
		"address": {&Grid{Style: "form", Label: "IPv4 address", Children: []Widget{
			&Field{Name: "ipaddr", Label: "Address", Key: "ipaddr"},
			&Field{Name: "netmask", Label: "Netmask", Key: "netmask"},
		}}, true},
		"window": {&Grid{Style: "form", Label: "Hours", Join: "to", Children: []Widget{
			&Field{Name: "start", Label: "Starts", Kind: "time"},
			&Field{Name: "stop", Label: "Ends", Kind: "time"},
		}}, true},
	} {
		got := render(t, r, w(c.w))
		if wide := strings.Contains(got, "verso-field-wide"); wide != c.wide {
			t.Errorf("%s: wide = %v, want %v:\n%s", name, wide, c.wide, got)
		}
	}
}

// w is a widget as a Widget, so a table of mixed widgets reads plainly.
func w(widget Widget) Widget { return widget }

// TestARefusalStandsUnderItsControl: a refusal is part of the answer, after
// the control and before the note, so it stands straight under the control
// whatever the description beside it says; it was once a grid row of its own,
// which a long description pushed down, opening a gap under the field.
func TestARefusalStandsUnderItsControl(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "synflood_burst", Label: "Burst", Key: "synflood_burst",
		Help:  "How many may arrive at once before the rate limit applies. Absorbs ordinary bursts, like a page opening 40 sockets.",
		Error: "Enter a whole number.", Note: "Counted in packets.",
	})
	question, answer := strings.Index(got, "verso-field-question"), strings.Index(got, "verso-field-answer")
	desc := strings.Index(got, "data-verso-field-desc")
	control := strings.Index(got, `id="synflood_burst" name="synflood_burst"`)
	refusal := strings.Index(got, `id="synflood_burst-error" data-verso-error`)
	note := strings.Index(got, `id="synflood_burst-note" data-verso-field-note`)
	if !(question < desc && desc < answer && answer < control && control < refusal && refusal < note) {
		t.Errorf("the refusal is not the answer's, between its control and its note:\n%s", got)
	}
	if strings.Count(got[answer:], "</div>") < 2 || strings.Index(got[answer:], "</div>") < strings.Index(got[answer:], "data-verso-error") {
		t.Errorf("the refusal stands outside the answer:\n%s", got)
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

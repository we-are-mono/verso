// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRowSwitchesCarryTheRowName: a bare switch has no visible label of its
// own, since the row around it is what it switches. A settings switch with no
// posted name has no id for its row's label to point at, so it carries the
// row's title itself, and a screen reader says which option it enables, not
// just "checkbox, checked".
func TestRowSwitchesCarryTheRowName(t *testing.T) {
	bare := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "SYN-flood protection", Toggle: &SettingsToggle{On: true}},
	}})
	if !strings.Contains(bare, `data-verso-switch aria-label="SYN-flood protection"`) {
		t.Errorf("a nameless settings switch takes the row's title as its name:\n%s", bare)
	}
}

// TestLabelledSwitchesKeepTheirLabel: where a real label already points at the
// checkbox (the form row, the conditional gate), an aria-label would override
// that label. The inline style shows words for both states, and an aria-label
// would pin it to one of them.
func TestLabelledSwitchesKeepTheirLabel(t *testing.T) {
	for _, style := range []string{"", "checkbox", "inline"} {
		got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", Style: style})
		if strings.Contains(got, `data-verso-switch id="enabled" name="enabled" aria-label`) {
			t.Errorf("style %q: a labelled switch must not carry an aria-label:\n%s", style, got)
		}
	}
	// A settings row's title is a real label for its switch, as a form row's is.
	settings := render(t, newRenderer(t), firewallDefaults())
	if !strings.Contains(settings, `<label for="synflood_protect" id="synflood_protect-label"`) ||
		strings.Contains(settings, `name="synflood_protect" aria-label`) {
		t.Errorf("a settings switch is named by its row's label, not an aria-label:\n%s", settings)
	}
}

// An editable settings value uses its visible title as the input's label.
func TestSettingsValueInputIsNamed(t *testing.T) {
	got := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "Cache size", Code: "cachesize", Value: "1000", Name: "cachesize"},
	}})
	if !strings.Contains(got, `for="cachesize"`) || !strings.Contains(got, `id="cachesize" type="text" name="cachesize" value="1000"`) {
		t.Errorf("the in-place value input must carry the row title:\n%s", got)
	}
}

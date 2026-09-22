// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRowSwitchesCarryTheRowName: a bare switch has no visible label of its
// own, since the row around it is what it switches. So the checkbox takes the
// row's name as its accessible name, and a screen reader says which rule it
// enables, not just "checkbox, checked".
func TestRowSwitchesCarryTheRowName(t *testing.T) {
	got := render(t, newRenderer(t), redirectsTable())
	if !strings.Contains(got, `name="force_dns_guest" aria-label="Force-DNS-to-AdGuard-guest"`) {
		t.Errorf("a table switch takes the row's comment as its name when no name column exists:\n%s", got)
	}

	named := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "#", Kind: "text"}, {Kind: "toggle"}, {Label: "Name", Kind: "name"}, {Label: "Note", Kind: "comment"}},
		Rows: []TableRow{{ID: "r1", Cells: []TableCell{
			{Text: "1"}, {Name: "r1_enabled", On: true}, {Text: "Allow-SSH"}, {Text: "from the office"},
		}}},
	})
	if !strings.Contains(named, `name="r1_enabled" aria-label="Allow-SSH"`) {
		t.Errorf("the name column wins over the order number and the comment:\n%s", named)
	}

	settings := render(t, newRenderer(t), firewallDefaults())
	if !strings.Contains(settings, `name="synflood_protect" aria-label="SYN-flood protection"`) {
		t.Errorf("a settings switch takes the row's title as its name:\n%s", settings)
	}
}

// TestLabelledSwitchesKeepTheirLabel: where a real label already points at the
// checkbox (the form row, the conditional gate), an aria-label would override
// that label. The inline style shows words for both states, and an aria-label
// would pin it to one of them.
func TestLabelledSwitchesKeepTheirLabel(t *testing.T) {
	for _, style := range []string{"", "checkbox", "inline"} {
		got := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", OffLabel: "Disabled", Style: style})
		if strings.Contains(got, `data-verso-switch id="enabled" name="enabled" aria-label`) {
			t.Errorf("style %q: a labelled switch must not carry an aria-label:\n%s", style, got)
		}
	}
}

// TestSettingsValueInputIsNamed: the always-open value box on a settings row
// sits in a label that holds only the input and a pencil, so the input takes
// the row's title as its name.
func TestSettingsValueInputIsNamed(t *testing.T) {
	got := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "Cache size", Code: "cachesize", Value: "1000", Name: "cachesize"},
	}})
	if !strings.Contains(got, `name="cachesize" value="1000" aria-label="Cache size"`) {
		t.Errorf("the in-place value input must carry the row title:\n%s", got)
	}
}

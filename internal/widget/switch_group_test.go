// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// includes is a firewall's included files: one setting — which of them load —
// asked of each file in turn.
func includes() *Grid {
	return &Grid{Style: "form", Columns: 1, Label: "Files the config includes", Children: []Widget{
		&Switch{Name: "include_a", Label: "/etc/nftables.d/10-custom.nft", Key: "enabled", Help: "nftables · chain-pre", Verbatim: true, On: true},
		&Switch{Name: "include_b", Label: "/etc/firewall.user", Key: "enabled", Help: "Shell script", Verbatim: true},
	}}
}

// TestSwitchesUnderOneLabelAreOneSetting: a labelled group of switches is one
// row — the label on top, the option chip once — holding a checkbox row per
// switch, each with its own words kept in view and its own change.
func TestSwitchesUnderOneLabelAreOneSetting(t *testing.T) {
	got := render(t, newRenderer(t), includes())
	if strings.Contains(got, "verso-form-grid") {
		t.Errorf("a labelled group of switches is one setting, not columns:\n%s", got)
	}
	for want, n := range map[string]int{
		`<span id="include_a-group-label" class="text-sm font-semibold text-ink">Files the config includes</span>`: 1,
		`>enabled</span>`:            1, // the option, named once for the group
		`class="verso-switch-group"`: 1,
		`data-verso-switch`:          2,
		"data-verso-change-field":    2, // each file is its own change
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	for _, want := range []string{
		`role="group" aria-labelledby="include_a-group-label"`,
		`<label for="include_a" id="include_a-label" class="font-mono text-base font-medium text-ink">/etc/nftables.d/10-custom.nft</label>`,
		`<p id="include_a-desc" data-verso-field-desc class="text-sm leading-5 text-pretty text-body">nftables · chain-pre</p>`,
		`aria-describedby="include_a-desc"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("switch group missing %q:\n%s", want, got)
		}
	}
}

// TestAGroupedSwitchWearsItsOwnMark: the rows are different things, so the one
// whose option waits is the one marked.
func TestAGroupedSwitchWearsItsOwnMark(t *testing.T) {
	g := includes()
	g.Children[1].(*Switch).Staged = true
	got := render(t, newRenderer(t), g)
	if n := strings.Count(got, "data-verso-staged-row"); n != 1 {
		t.Fatalf("want one mark, got %d:\n%s", n, got)
	}
	if at := strings.Index(got, "data-verso-staged-row"); !strings.Contains(got[:at], "/etc/firewall.user") {
		t.Errorf("the mark stands on the row whose option waits:\n%s", got)
	}
}

// TestALockedSwitchStatesAFixedState: a state nothing here can change is drawn
// as the checkbox it would be, set and inert, and posts nothing.
func TestALockedSwitchStatesAFixedState(t *testing.T) {
	got := render(t, newRenderer(t), &Switch{Name: "always", Label: "/etc/nftables.d/", Style: "locked", On: true, Verbatim: true})
	for _, want := range []string{`data-verso-switch id="always"`, " disabled", " checked", "peer-disabled:"} {
		if !strings.Contains(got, want) {
			t.Errorf("locked switch missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "data-verso-change-field") || strings.Contains(got, `name="always"`) {
		t.Errorf("a locked switch posts nothing and tracks no change:\n%s", got)
	}
}

// TestASwitchIsRefusedLikeAField: a switch the plugin refuses wears the same
// refusal a field does — the crimson band, under the label it refuses, named
// by the checkbox — not a notice of its own beside the row.
func TestASwitchIsRefusedLikeAField(t *testing.T) {
	s := &Switch{Name: "dnssec", Label: "Verify answers are signed", Key: "dnssec", Style: "checkbox", On: true,
		Error: "DNSSEC requires a dnsmasq build with DNSSEC support."}
	got := render(t, newRenderer(t), s)
	for _, want := range []string{
		`<span id="dnssec-error" data-verso-error class="flex items-start gap-2 rounded-xs bg-crimson-soft px-3 py-2.5 text-sm leading-5 text-crimson-deep">`,
		`aria-describedby="dnssec-error"`,
		`aria-invalid="true"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("refused switch missing %q:\n%s", want, got)
		}
	}
	translateSchema(s, func(w string) string { return "«" + w + "»" })
	if s.Error != "«DNSSEC requires a dnsmasq build with DNSSEC support.»" {
		t.Errorf("a switch's refusal is prose and translates: %q", s.Error)
	}
}

// TestAVerbatimLabelStaysAsWritten: a label that is a machine string is never
// looked up in a catalog.
func TestAVerbatimLabelStaysAsWritten(t *testing.T) {
	g := includes()
	translateSchema(g, func(s string) string { return "«" + s + "»" })
	if s := g.Children[0].(*Switch); s.Label != "/etc/nftables.d/10-custom.nft" || s.Help != "«nftables · chain-pre»" {
		t.Errorf("verbatim label translated or help left alone: %q %q", s.Label, s.Help)
	}
	if g.Label != "«Files the config includes»" {
		t.Errorf("the group's label is words and translates: %q", g.Label)
	}
}

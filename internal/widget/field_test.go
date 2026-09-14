// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderFieldChecks: the checks kind is membership in a known set — every
// box shares the field's name (posting multi-value, the list contract), the
// current values come back checked, and the rest don't.
func TestRenderFieldChecks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{
		Name: "network", Label: "Networks", Kind: "checks",
		Values: []string{"lan", "lan2"},
		Options: []Option{
			{Value: "lan", Label: "lan"}, {Value: "lan2", Label: "lan2"}, {Value: "guest", Label: "guest"},
		},
	})
	if strings.Count(got, `<input type="checkbox" name="network"`) != 3 {
		t.Errorf("all boxes must share the field name:\n%s", got)
	}
	if strings.Count(got, " checked") != 2 {
		t.Errorf("exactly the current values should be checked:\n%s", got)
	}
	for _, want := range []string{`data-verso-change-name="network"`, `data-verso-change-label="Networks"`, `type="checkbox"`, ">Networks<", "guest"} {
		if !strings.Contains(got, want) {
			t.Errorf("checks field missing %q:\n%s", want, got)
		}
	}
}

func TestRenderFieldDateTimeLocal(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "datetime", Label: "Date and time", Kind: "datetime-local",
		Value: "2026-08-29T22:14:08",
	})
	for _, want := range []string{
		`type="datetime-local"`, `step="1"`, `value="2026-08-29T22:14:08"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("datetime-local field missing %q:\n%s", want, got)
		}
	}
}

// TestRenderFieldTip: a label carrying an explanation becomes the thing you
// hover — help cursor, its own tab stop, and the control pointed at it — and the
// tooltip ends on the option and what reads it.
func TestRenderFieldTip(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "duid", Label: "DUID", Value: "", Key: "duid", Source: "dhcp host",
		Tip:  "A DHCPv6 client identifies itself by a DUID, not by its MAC.",
		Help: "Match a DHCPv6 client by DUID instead of MAC.",
	})
	for _, want := range []string{
		"group/tip", "cursor-help", `tabindex="0"`, `aria-describedby="duid-tip"`,
		`id="duid-tip"`, `role="tooltip"`, "group-hover/tip:opacity-100",
		// The hook verso.js measures against to flip a tip with no room below.
		"data-verso-tip",
		"A DHCPv6 client identifies itself by a DUID, not by its MAC.",
		"duid · dhcp host",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("explained field missing %q:\n%s", want, got)
		}
	}
	// The row is one line and its control. An explanation lives on the label
	// that raises it, never as a second line under the control: a sentence
	// per field pushes the rows apart until a form of eight settings reads as
	// a page of prose. Tip is the deliberate sentence, so it wins over Help.
	if strings.Contains(got, "Match a DHCPv6 client by DUID instead of MAC.") {
		t.Errorf("help must not take a line of its own beside a tip:\n%s", got)
	}
}

// TestRenderFieldHelpRaised: a plugin that only wrote Help still gets the
// explanation raised onto the label — the same sentence, said where it belongs,
// without every plugin having to rename the field it already fills in.
func TestRenderFieldHelpRaised(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "leasetime", Label: "Lease time", Key: "leasetime",
		Help: "How long a client may keep an address before it asks again.",
	})
	for _, want := range []string{
		"group/tip", `id="leasetime-tip"`, `role="tooltip"`,
		"How long a client may keep an address before it asks again.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help-only field missing %q:\n%s", want, got)
		}
	}
}

// TestRenderFieldWithoutTip: a label with nothing to explain stays inert — no
// hover affordance, no tab stop of its own, no tooltip.
func TestRenderFieldWithoutTip(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "name", Label: "Name", Key: "name"})
	for _, unwanted := range []string{"group/tip", "cursor-help", `tabindex="0"`, "role=\"tooltip\""} {
		if strings.Contains(got, unwanted) {
			t.Errorf("plain field should not carry %q:\n%s", unwanted, got)
		}
	}
}

// TestFieldTipFooterHalves: the footer is whichever halves it has, so an option
// named without a reader still places itself and neither renders a bare dot.
func TestFieldTipFooterHalves(t *testing.T) {
	for _, tc := range []struct{ key, source, want string }{
		{"duid", "dhcp host", "duid · dhcp host"},
		{"duid", "", "duid"},
		{"", "dhcp host", "dhcp host"},
		{"", "", ""},
	} {
		f := &Field{Name: "duid", Tip: "…", Key: tc.key, Source: tc.source}
		if got := f.TipView().Footer; got != tc.want {
			t.Errorf("footer for key %q source %q = %q, want %q", tc.key, tc.source, got, tc.want)
		}
	}
}

func TestRenderFieldCurrentPasswordAutocomplete(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "current_password", Label: "Current password", Kind: "password", Autocomplete: "current-password",
	})
	if !strings.Contains(got, `autocomplete="current-password"`) {
		t.Errorf("current password autocomplete missing:\n%s", got)
	}
}

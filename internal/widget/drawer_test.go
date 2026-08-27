// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestDrawerDefaults: the reading-width panel and the framed card trigger.
func TestDrawerDefaults(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Drawer{Title: "Detail", Trigger: []Widget{&Row{Title: "open me"}}})
	for _, want := range []string{"max-w-md", "rounded-xl", "open me", "Detail"} {
		if !strings.Contains(got, want) {
			t.Errorf("drawer missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "max-w-2xl") {
		t.Error("default drawer must not be wide")
	}
}

// TestDrawerWideBare: the wide panel for detail views, the bare trigger for
// rows living in a hairline-divided list — no card frame, the row hover tint.
func TestDrawerWideBare(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Drawer{Title: "Device", Size: "wide", Style: "bare",
		Trigger: []Widget{&Row{Title: "family-laptop", Chevron: true}}})
	for _, want := range []string{"max-w-2xl", "hover:bg-slate-50", chevronPath} {
		if !strings.Contains(got, want) {
			t.Errorf("wide bare drawer missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "rounded-xl") {
		t.Error("bare trigger must not wear the card frame")
	}
}

// chevronPath is the chevron-right icon's path data — the icon renders as
// inline SVG, so its name never appears in output.
const chevronPath = "m9 18 6-6-6-6"

// TestRowChevronIsOptIn: a plain informational row carries no "this opens"
// cue; a trigger row asks for it explicitly.
func TestRowChevronIsOptIn(t *testing.T) {
	r := newRenderer(t)
	if got := render(t, r, &Row{Title: "plain"}); strings.Contains(got, chevronPath) {
		t.Errorf("plain row must not render a chevron:\n%s", got)
	}
	if got := render(t, r, &Row{Title: "opens", Chevron: true}); !strings.Contains(got, chevronPath) {
		t.Errorf("trigger row missing its chevron:\n%s", got)
	}
}

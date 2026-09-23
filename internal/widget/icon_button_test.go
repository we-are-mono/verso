// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// iconButton is the one dress of a glyph-only act: a 28px square in Glyph on
// no border, taking the parked border, the hairline wash and Ink under the
// pointer — a table row's edit or remove, a list's remove, a copy beside a
// value or on a code box.
const iconButton = "size-7 shrink-0 cursor-pointer place-items-center rounded-xs border border-transparent text-glyph transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink"

// buttonClass returns the class of the first button after marker.
func buttonClass(t *testing.T, html, marker string) string {
	t.Helper()
	at := strings.Index(html, marker)
	if at < 0 {
		t.Fatalf("no %q in:\n%s", marker, html)
	}
	open := strings.LastIndex(html[:at], "<button")
	if open < 0 {
		t.Fatalf("%q is not on a button:\n%s", marker, html)
	}
	tag := html[open:]
	return tag[:strings.Index(tag, ">")]
}

// TestEveryIconActLooksAlike: a list's remove, a copy beside a fact, a copy
// beside a table's address and a code box's copy wear the table's icon act.
func TestEveryIconActLooksAlike(t *testing.T) {
	r := newRenderer(t)
	for name, tag := range map[string]string{
		"list remove": buttonClass(t, render(t, r, &List{Name: "server", Label: "Time servers", Style: "rows", Items: []string{"0.pool.ntp.org"}}), "data-verso-list-remove"),
		"fact copy":   buttonClass(t, render(t, r, &Properties{Items: []Property{{Label: "Fingerprint", Value: "AB:CD", Mono: true, Copy: true}}}), `x-data="copy"`),
		"code copy":   buttonClass(t, render(t, r, &Code{Value: "ssh-ed25519 AAAA", Copy: true}), `x-data="copy"`),
	} {
		if !strings.Contains(tag, iconButton) {
			t.Errorf("%s is not the icon act: %s", name, tag)
		}
	}
}

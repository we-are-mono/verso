// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestAWithdrawnLinkIsNotDrawn: a link the shell withdrew (one into a plugin
// that is not installed) is drawn nowhere: not on its own, not as a section's
// act, not as a notice's door or one of its acts, so nothing on the page leads
// to an address with nothing at it.
func TestAWithdrawnLinkIsNotDrawn(t *testing.T) {
	r := newRenderer(t)
	gone := func() *Link {
		l := &Link{Label: "Configure DHCP", Href: "/plugins/dnsdhcp/?open=lan", Style: "act"}
		l.Withdraw()
		return l
	}
	if got := render(t, r, gone()); got != "" {
		t.Errorf("a withdrawn link is drawn:\n%s", got)
	}
	section := render(t, r, &Section{Title: "DHCP server", Control: gone(), Children: []Widget{&Text{Markdown: "Serving"}}})
	if strings.Contains(section, "Configure DHCP") || strings.Contains(section, "dnsdhcp") || !strings.Contains(section, "Serving") {
		t.Errorf("a section keeps a withdrawn act, or loses its words:\n%s", section)
	}
	notice := render(t, r, &Callout{Body: "Leases come from the DHCP server.", Link: gone(), Acts: []Link{*gone(), {Label: "Open Interfaces", Href: "/plugins/interfaces/"}}})
	if strings.Contains(notice, "dnsdhcp") || !strings.Contains(notice, `href="/plugins/interfaces/"`) {
		t.Errorf("a notice keeps a withdrawn door or act, or loses its others:\n%s", notice)
	}
	if kept := render(t, r, &Link{Label: "Open", Href: "/plugins/dnsdhcp/"}); !strings.Contains(kept, `href="/plugins/dnsdhcp/"`) {
		t.Errorf("a link nobody withdrew is not drawn:\n%s", kept)
	}
}

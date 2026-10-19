// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestALinkElsewhereKeepsItsAnchor: a link to a section of a page in the
// shell names it by the shell's section id; a link to another site keeps the
// anchor that site wrote.
func TestALinkElsewhereKeepsItsAnchor(t *testing.T) {
	r := newRenderer(t)
	if got := render(t, r, &Link{Label: "Guide", Href: "https://example.org/guide#adding-a-board"}); !strings.Contains(got, `href="https://example.org/guide#adding-a-board"`) {
		t.Errorf("a link elsewhere lost its anchor:\n%s", got)
	}
	if got := render(t, r, &Link{Label: "DHCP", Href: "/plugins/interfaces/#dhcp"}); !strings.Contains(got, `href="/plugins/interfaces/#`+SectionID("dhcp")+`"`) {
		t.Errorf("a link to a section in the shell is not its section id:\n%s", got)
	}
}

// TestALinkIsTheLinkBlock: a link that stands on its own is the link block
// (blocks/link.css), in the action colour; one that closes a notice's words or
// stands in a row's value is a door, which the shell marks so it takes the
// words' own ink and weight and is told apart only by its underline.
func TestALinkIsTheLinkBlock(t *testing.T) {
	r := newRenderer(t)
	if got := render(t, r, &Link{Label: "Project website", Href: "/about"}); !strings.Contains(got, `<a href="/about" class="verso-link">`) {
		t.Errorf("a link is not the link block:\n%s", got)
	}
	notice := render(t, r, &Callout{Variant: "warning", Body: "The zone accepts everything.", Link: &Link{Label: "Open Firewall", Href: "/plugins/firewall/"}})
	if !strings.Contains(notice, `<a href="/plugins/firewall/" class="verso-link" data-verso-door>`) {
		t.Errorf("a notice's door is not marked a door:\n%s", notice)
	}
	cell := render(t, r, &Table{
		Columns: []TableColumn{{Label: "Rule", Kind: "phrase"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "see the zone", Parts: []TablePart{{Words: "see"}, {Href: "/x", Words: "the zone"}}}}}},
	})
	if !strings.Contains(cell, `<a href="/x" class="verso-link verso-table__part" data-verso-door>`) {
		t.Errorf("a link in a row's words is not a door:\n%s", cell)
	}
}

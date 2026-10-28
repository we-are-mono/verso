// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestActLinkOpensItsFormAsAPanel: a subsection act that makes or replaces
// the thing it stands under (a certificate's Install, Make a new one) opens
// its form in a drawer over the page, in the act's own dress. The page keeps
// its address; an act without a panel stays a plain link.
func TestActLinkOpensItsFormAsAPanel(t *testing.T) {
	got := render(t, newRenderer(t), &Link{Style: "act", Label: "Make a new one", Icon: "refresh-cw",
		Href: "/plugins/system/access/certificate/new", Panel: true})
	for _, want := range []string{
		`x-data="modal"`, `@click.prevent="showPanel"`, "data-verso-panel-chrome",
		"x-teleport", "data-verso-panel", `href="/plugins/system/access/certificate/new"`,
		lucideIcons["refresh-cw"], ">Make a new one</a>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	plain := render(t, newRenderer(t), &Link{Style: "act", Label: "Download", Icon: "download", Href: "/system/access/certificate"})
	if strings.Contains(plain, "x-data") || strings.Contains(plain, "showPanel") {
		t.Errorf("an act without a panel is a plain link:\n%s", plain)
	}
}

// TestAnActLinkToAPlaceOfThisPageMovesTheAddress: an act that makes one of
// the things this page lists (a card's "Add reservation") opens a place of
// the page, not a thing of its own: the address follows the panel, as a
// row's drawer's and the heading act's do, so the panel is not the chrome's.
// It stands on a card's header beside the title, as any section's act does.
func TestAnActLinkToAPlaceOfThisPageMovesTheAddress(t *testing.T) {
	act := &Link{Style: "act", Label: "Add reservation", Icon: "plus", Href: "/plugins/dnsdhcp/?open=new", Panel: true, Place: true}
	got := render(t, newRenderer(t), &Section{Title: "Reservations", Control: act, Children: []Widget{&Text{Markdown: "a"}}})
	for _, want := range []string{
		`<section class="verso-card" data-verso-section="card">`, `<span class="verso-heading__act">`,
		`x-data="modal"`, `@click.prevent="showPanel"`, `href="/plugins/dnsdhcp/?open=new"`, lucideIcons["plus"], ">Add reservation</a>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	head, title, link := strings.Index(got, "verso-card__head"), strings.Index(got, ">Reservations<"), strings.Index(got, ">Add reservation</a>")
	if head < 0 || title < head || link < title || link > strings.Index(got, "verso-card__body") {
		t.Errorf("the act does not stand on the card's header after its title:\n%s", got)
	}
	if strings.Contains(got, "data-verso-panel-chrome") {
		t.Errorf("a place of this page is not the chrome's panel:\n%s", got)
	}
}

// TestASecondaryLinkOpensItsDestinationAsAPanel: a secondary act that installs
// a package opens that package's drawer over the page, as an act does.
func TestASecondaryLinkOpensItsDestinationAsAPanel(t *testing.T) {
	got := render(t, newRenderer(t), &Link{Style: "secondary", Label: "Install adblock", Icon: "download",
		Href: "/system/packages/package?name=adblock", Panel: true})
	for _, want := range []string{`x-data="modal"`, `@click.prevent="showPanel"`, "data-verso-panel-chrome", "data-verso-panel", ">Install adblock</a>"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

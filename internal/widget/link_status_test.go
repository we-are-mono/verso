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

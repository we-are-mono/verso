// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestStatusLinkIsASettingsRow: a status among settings is a settings row — the
// hollow packet and the fact where a setting's name stands, and the act that
// changes it where a setting's control stands — not a band ruled off above and
// below. Its act is a field-height quiet button that names itself.
func TestStatusLinkIsASettingsRow(t *testing.T) {
	got := render(t, newRenderer(t), &Link{Style: "status", Label: "Queries leave in plain text",
		Act: "Install https-dns-proxy", Icon: "download", Href: "/system/packages/package?name=https-dns-proxy"})
	for _, want := range []string{
		`class="verso-field-row max-w-form"`,
		`<span aria-hidden="true" class="size-1.5 shrink-0 rounded-[1px] border border-faint"></span>`,
		`<span class="text-sm font-semibold text-ink">Queries leave in plain text</span>`,
		`href="/system/packages/package?name=https-dns-proxy"`,
		">Install https-dns-proxy</a>",
		lucideIcons["download"],
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "border-y") || strings.Contains(got, "x-data") {
		t.Errorf("a plain status row is neither a ruled band nor a panel:\n%s", got)
	}
}

// TestStatusLinkOpensItsActAsAPanel: an act that is its own thing — a package
// to install — opens over the page in the drawer that thing has everywhere,
// rather than leaving the page. The page's address stays the page's: the
// panel is not a place on it.
func TestStatusLinkOpensItsActAsAPanel(t *testing.T) {
	got := render(t, newRenderer(t), &Link{Style: "status", Label: "No blocklist",
		Act: "Install adblock", Icon: "download", Href: "/system/packages/package?name=adblock", Panel: true})
	for _, want := range []string{
		`x-data="modal"`, `@click.prevent="showPanel"`, "data-verso-panel-chrome",
		"x-teleport", "data-verso-panel",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

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

// TestAStatusAnotherProgramOwnsOffersNoAct: a fact owned by something else on
// the router (AdGuard Home handling privacy) is said, and nothing is offered
// to change it here.
func TestAStatusAnotherProgramOwnsOffersNoAct(t *testing.T) {
	got := render(t, newRenderer(t), &Link{Style: "status", Label: "Handled by AdGuard Home",
		Desc: "AdGuard Home on this router filters what your devices look up."})
	if !strings.Contains(got, ">Handled by AdGuard Home</span>") || strings.Contains(got, "<a ") {
		t.Errorf("a status with no act draws no button:\n%s", got)
	}
	// The machine's name for what the fact is about rides after it as a key
	// chip, as a setting's option does.
	front := render(t, newRenderer(t), &Link{Style: "status", Label: "AdGuard Home answers your devices", Code: "AdGuardHome"})
	if !strings.Contains(front, `answers your devices</span><span class="verso-chip`) || !strings.Contains(front, ">AdGuardHome</span>") {
		t.Errorf("a status's code rides after its label as a chip:\n%s", front)
	}
}

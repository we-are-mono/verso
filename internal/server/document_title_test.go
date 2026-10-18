// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestDocumentTitleNamesThePage: a tab, a history entry and a screen reader's
// first words all read the document title, so it names the page, the most
// specific part first. Home's heading is a status sentence, not a name, so home
// is titled by what it is.
func TestDocumentTitleNamesThePage(t *testing.T) {
	cases := []struct {
		overview        bool
		heading, detail string
		want            string
	}{
		{false, "Access", "", "Access · Verso"},
		{false, "Firewall", "Rules", "Rules · Firewall · Verso"},
		{false, "Firewall", "Firewall", "Firewall · Verso"},
		{false, "Firewall rules", "Rules", "Firewall rules · Verso"},
		{true, "Internet is working", "", "Overview · Verso"},
		{false, "", "", "Verso"},
	}
	for _, c := range cases {
		if got := documentTitle(c.overview, c.heading, c.detail, nil); got != c.want {
			t.Errorf("documentTitle(%v, %q, %q) = %q, want %q", c.overview, c.heading, c.detail, got, c.want)
		}
	}
}

// TestAccessContributionStandsOffByThePageGap: a plugin's part of Access is
// one more of the page's subjects, set into the page's own stack (an Embed),
// so no wrapper of its own spaces it. The stack stands the rule it opens on two
// cells under what is above it, as every section's rule stands — its margin
// those cells less the rule's pixel, so the rule lands on a line.
func TestAccessContributionStandsOffByThePageGap(t *testing.T) {
	css, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), `.verso-page__body > .verso-stack > * + .verso-stack:has(> section[data-verso-section]:first-child > [data-verso-section-band]) { margin-top: calc(var(--spacing) * 10 - 1px); }`) {
		t.Error("a part that opens on a section's rule does not land the rule two cells under the part before it")
	}
	if _, err := os.Stat("templates/access-contribution.html.tmpl"); err == nil {
		t.Error("a plugin's part of Access is drawn through a hand-built wrapper")
	}
}

// TestPageMastheadStandsOnTheSand: the heading's line stands 24px under the
// top bar and 24px over the first card, with no bar or hairline of its own.
// The masthead still aligns page acts.
func TestPageMastheadStandsOnTheSand(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	access := get(t, s, "/system/access").Body.String()
	if !strings.Contains(access, `<div data-verso-masthead class="verso-page__masthead">`) {
		t.Error("the heading's line stands 24px clear of the top bar and of the first card")
	}
	masthead := cssBlock(t, blockSheet(t, "page"), "\n.verso-page__masthead {")
	if !strings.Contains(masthead, "padding: calc(var(--spacing) * 6) ") || !strings.Contains(masthead, " calc(var(--spacing) * 6) var(--verso-gutter);") {
		t.Errorf("the heading's line stands 24px clear of the top bar and of the first card:\n%s", masthead)
	}
	for _, own := range []string{"border", "background"} {
		if strings.Contains(masthead, own) {
			t.Errorf("the masthead on the sand draws no %s of its own:\n%s", own, masthead)
		}
	}
	if strings.Contains(access, "data-verso-bleed") {
		t.Error("the page's rules are no one page's experiment")
	}
}

// TestServerPagesNameEachThingOnce: an id names one element. Two with one
// name send a label, an error, or the rail's link to whichever the browser
// finds first — Maintenance's restore box and its section once shared
// "backup", so the rail's "Back up and restore" pointed at a hidden input.
func TestServerPagesNameEachThingOnce(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	ids := regexp.MustCompile(`\sid="([^"]+)"`)
	for _, path := range []string{"/system/maintenance", "/system/access"} {
		body := get(t, s, path).Body.String()
		seen := map[string]int{}
		for _, m := range ids.FindAllStringSubmatch(body, -1) {
			seen[m[1]]++
		}
		for id, n := range seen {
			if n > 1 {
				t.Errorf("%s names %d elements %q", path, n, id)
			}
		}
	}
}

// TestMaintenanceSectionsUsePageBands: the masthead stands on the sand, and
// Maintenance is the shell's widgets alone — its sections the section widget's
// cards, the later ones ruled, drawn by no hand-built template — so each band's
// rule stands two cells under the content before it, as its title stands two
// cells under the rule.
func TestMaintenanceSectionsUsePageBands(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	if !strings.Contains(body, `<div data-verso-masthead class="verso-page__masthead">`) {
		t.Error("the masthead opens onto the first section from the sand")
	}
	// With nothing to say, nothing stands before Firmware: the section opens
	// the work column right under the masthead's line.
	if !regexp.MustCompile(`data-verso-form-column class="verso-grid__cell">\s*<div class="verso-stack">\s*<section data-verso-section="card" class="verso-card" id="section-firmware">`).MatchString(body) {
		t.Error("something stands before Firmware with nothing to say")
	}
	for _, id := range []string{"back-up-and-restore", "reboot", "factory-reset"} {
		want := `<section data-verso-section="card" class="verso-card" id="section-` + id + `" data-verso-ruled data-verso-section-divider>`
		if !strings.Contains(body, want) {
			t.Errorf("want %s", want)
		}
	}
	if _, err := os.Stat("templates/maintenance.html.tmpl"); err == nil {
		t.Error("Maintenance is drawn by a hand-built template")
	}
}

// TestMaintenanceHeadingsAreSectionHeadings: the page drawn by hand heads its
// sections exactly as the section widget does — one band, one title, one
// lede — so the two cannot drift apart.
func TestMaintenanceHeadingsAreSectionHeadings(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	band := `<div data-verso-section-band class="verso-card__head flex flex-wrap items-center justify-between gap-x-6">`
	title := `<h2 class="text-lg leading-6 font-semibold tracking-[-0.025em] text-ink">`
	lede := `<div class="w-full"><div data-verso-section-lede class="verso-prose text-body">`
	for want, n := range map[string]int{band: 4, title: 4, lede: 2} {
		if got := strings.Count(body, want); got < n {
			t.Errorf("want at least %d of %q, got %d", n, want, got)
		}
	}
	if strings.Contains(body, `data-verso-section-band class="mb-5 flex-col gap-1"`) || strings.Contains(body, `<p data-verso-section-lede`) {
		t.Error("no heading is drawn by hand any more")
	}
	// What stands on a heading's line is a section meta: its label in the
	// meta ink, its value set in the body ink at 600.
	meta := `<span class="text-meta">Running for</span><span data-verso-section-meta class="font-semibold text-body">`
	if !strings.Contains(body, meta) {
		t.Errorf("the reboot heading's uptime is a section meta, want %q", meta)
	}
}

func TestAccessPageIsTitledAccess(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/access").Body.String()
	if !strings.Contains(body, "<title>Access · Verso</title>") {
		t.Errorf("want the page named in its title, got %q", body[strings.Index(body, "<title>"):strings.Index(body, "</title>")+8])
	}
}

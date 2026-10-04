// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"html/template"
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
// one more of the page's subjects. Its heading band keeps the standard 24px
// gap, without an extra divider or padding before it.
func TestAccessContributionStandsOffByThePageGap(t *testing.T) {
	var b bytes.Buffer
	if err := passwordServer(t, fakeBackend{}).pageSet("").ExecuteTemplate(&b, "access-contribution.html.tmpl", template.HTML("<section>SSH</section>")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<div class="mt-6"><section>SSH</section></div>`) {
		t.Errorf("want a 24px gap without a divider before the contribution, got %s", b.String())
	}
}

// TestPageMastheadStandsOnAHairline: the heading's line stands 16px over its
// own hairline, as far as it stands under the bar's top, and the page 24px
// under it. The masthead still aligns page acts.
func TestPageMastheadStandsOnAHairline(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	access := get(t, s, "/system/access").Body.String()
	if !strings.Contains(access, `<div data-verso-masthead class="mb-6 py-4">`) {
		t.Error("the masthead stands 16px over its hairline and the page 24px under it")
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

// TestMaintenanceSectionsUsePageBands: the masthead stands on its hairline, and
// Maintenance's content keeps 32px before the next band to match the 32px
// between the preceding band and its first content.
func TestMaintenanceSectionsUsePageBands(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	if !strings.Contains(body, `<div data-verso-masthead class="mb-6 py-4">`) {
		t.Error("the masthead opens onto the first section from its hairline")
	}
	// With nothing to say, nothing stands before Firmware: the section opens
	// the page right under the masthead's line.
	if !regexp.MustCompile(`max-w-form shrink-0">\s*<section data-verso-section="ruled" id="firmware">`).MatchString(body) {
		t.Error("an empty notice block stands before Firmware")
	}
	for _, id := range []string{"back-up-and-restore", "reboot", "factory-reset"} {
		// The same ruled section the widget draws; its air is sections.css's.
		want := `<section data-verso-section="ruled" data-verso-ruled data-verso-section-divider id="` + id + `">`
		if !strings.Contains(body, want) {
			t.Errorf("want %s", want)
		}
	}
	if strings.Contains(body, `<section id="firmware" class="scroll-mt-20 py-8"`) || strings.Contains(body, "border-rule py-8") {
		t.Error("no section keeps the old 32px step")
	}
}

// TestMaintenanceHeadingsAreSectionHeadings: the page drawn by hand heads its
// sections exactly as the section widget does — one band, one title, one
// lede — so the two cannot drift apart.
func TestMaintenanceHeadingsAreSectionHeadings(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	band := `<div data-verso-section-band class="mb-5 flex flex-wrap items-center justify-between gap-x-6 gap-y-1">`
	title := `<h2 class="text-lg leading-tight font-semibold tracking-[-0.025em] text-ink">`
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

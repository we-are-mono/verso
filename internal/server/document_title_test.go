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
// one more of the page's subjects, so its rule keeps the page's 40 on both
// sides, as the shell's own ruled sections do.
func TestAccessContributionStandsOffByThePageGap(t *testing.T) {
	var b bytes.Buffer
	if err := passwordServer(t, fakeBackend{}).pageSet("").ExecuteTemplate(&b, "access-contribution.html.tmpl", template.HTML("<section>SSH</section>")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<div data-verso-rule class="mt-6 border-t border-rule pt-6"><section>SSH</section></div>`) {
		t.Errorf("want 40 above and below the seam, got %s", b.String())
	}
}

// TestSectionRulesAreThePagesAcrossItsGutter: a page's rules between its
// sections run out by the frame's gutter to the rail on the left, and stop
// where the column ends on the right, on every page, so the masthead's
// rule and a plugin's seam are marked as rules of the page; the stylesheet
// draws them out, and keeps a drawer's own rules inside the drawer.
func TestSectionRulesAreThePagesAcrossItsGutter(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	access := get(t, s, "/system/access").Body.String()
	if !strings.Contains(access, `<div data-verso-rule data-verso-masthead class="mb-6 border-b border-rule pb-5`) {
		t.Error("the masthead's rule is one of the page's")
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

// TestMaintenanceSectionsAreRuledAsThePages: Maintenance divides its sections
// as every page does — the masthead ruled off, each later section 24px after
// the last and 24px under its rule, the rules the page's own.
func TestMaintenanceSectionsAreRuledAsThePages(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	if !strings.Contains(body, `<div data-verso-rule data-verso-masthead class="mb-6 border-b border-rule pb-5`) {
		t.Error("the masthead is ruled off from the first section")
	}
	for _, id := range []string{"back-up-and-restore", "reboot", "factory-reset"} {
		want := `<section id="` + id + `" data-verso-rule class="scroll-mt-20 mt-6 border-t border-rule pt-6">`
		if !strings.Contains(body, want) {
			t.Errorf("want %s", want)
		}
	}
	if strings.Contains(body, `<section id="firmware" class="scroll-mt-20 py-8"`) || strings.Contains(body, "border-rule py-8") {
		t.Error("no section keeps the old 32px step")
	}
}

func TestAccessPageIsTitledAccess(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/access").Body.String()
	if !strings.Contains(body, "<title>Access · Verso</title>") {
		t.Errorf("want the page named in its title, got %q", body[strings.Index(body, "<title>"):strings.Index(body, "</title>")+8])
	}
}

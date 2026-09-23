// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"html/template"
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
	if !strings.Contains(b.String(), `<div class="mt-10 border-t border-rule pt-10"><section>SSH</section></div>`) {
		t.Errorf("want 40 above and below the seam, got %s", b.String())
	}
}

func TestAccessPageIsTitledAccess(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/access").Body.String()
	if !strings.Contains(body, "<title>Access · Verso</title>") {
		t.Errorf("want the page named in its title, got %q", body[strings.Index(body, "<title>"):strings.Index(body, "</title>")+8])
	}
}

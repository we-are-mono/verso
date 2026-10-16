// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFoldedDescriptionsArriveFolded: a browser that folded the descriptions
// away gets every page with them folded from its first paint (form-fields):
// the page root says so. Any other word in the cookie, or none, is the
// default, descriptions in view.
func TestFoldedDescriptionsArriveFolded(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for value, folded := range map[string]bool{"folded": true, "shown": false, "": false, `folded" onload="x`: false} {
		req := httptest.NewRequest(http.MethodGet, "/system/maintenance", nil)
		if value != "" {
			req.AddCookie(&http.Cookie{Name: descriptionsCookie, Value: value})
		}
		body := serve(t, s, req).Body.String()
		if got := strings.Contains(body, `<html lang="en" data-verso-descriptions="folded">`); got != folded {
			t.Errorf("cookie %q: folded root %v, want %v", value, got, folded)
		}
		if !folded && strings.Contains(body, "data-verso-descriptions=") {
			t.Errorf("cookie %q: the root carries a fold it was not asked for", value)
		}
	}
}

// TestTheHeadingLineOffersTheFold: every page's heading line carries the act
// that folds the descriptions away and back, with both its words and glyphs,
// and both its words are in the Slovenian catalog.
func TestTheHeadingLineOffersTheFold(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	masthead := strings.Index(body, "data-verso-masthead")
	page := strings.Index(body, `class="verso-page-body"`)
	if masthead < 0 || page < masthead {
		t.Fatalf("no masthead before the page body:\n%s", body)
	}
	line := body[masthead:page]
	for _, want := range []string{"data-verso-fold", ">Hide descriptions<", ">Show descriptions<", `<path d="m15 5-3 3-3-3"`, `<path d="m15 5-3-3-3 3"`} {
		if !strings.Contains(line, want) {
			t.Errorf("the heading line lacks %q:\n%s", want, line)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "i18n", "sl", "base.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]string
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	for en, sl := range map[string]string{"Hide descriptions": "Skrij opise", "Show descriptions": "Prikaži opise"} {
		if catalog[en] != sl {
			t.Errorf("the Slovenian catalog says %q for %q, want %q", catalog[en], en, sl)
		}
	}
}

// TestTheSignInPageIgnoresTheFold: the sign-in page has no descriptions to
// fold and says nothing of the cookie.
func TestTheSignInPageIgnoresTheFold(t *testing.T) {
	s := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: descriptionsCookie, Value: "folded"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "data-verso-descriptions") {
		t.Errorf("the sign-in page takes the fold:\n%s", rec.Body.String())
	}
}

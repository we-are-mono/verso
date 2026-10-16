// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
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

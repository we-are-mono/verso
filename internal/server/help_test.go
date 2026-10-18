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

// TestShownHelpArrivesShown: a browser that showed the settings' help gets
// every page with it shown from its first paint (form-fields): the page root
// says so. Any other word in the cookie, or none, is the default, help
// folded.
func TestShownHelpArrivesShown(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for value, shown := range map[string]bool{"shown": true, "folded": false, "": false, `shown" onload="x`: false} {
		req := httptest.NewRequest(http.MethodGet, "/system/maintenance", nil)
		if value != "" {
			req.AddCookie(&http.Cookie{Name: helpCookie, Value: value})
		}
		root := rootTag(serve(t, s, req).Body.String())
		if got := root == `<html lang="en" data-verso-help="shown">`; got != shown {
			t.Errorf("cookie %q: shown root %v, want %v: %s", value, got, shown, root)
		}
		if !shown && strings.Contains(root, "data-verso-help") {
			t.Errorf("cookie %q: the root shows help it was not asked for: %s", value, root)
		}
	}
}

// rootTag is a page's <html> tag: the page's own state, apart from the
// stylesheet inlined under it, which names every state it styles.
func rootTag(body string) string {
	at := strings.Index(body, "<html")
	if at < 0 {
		return ""
	}
	return body[at : at+strings.Index(body[at:], ">")+1]
}

// TestTheHeadingLineOffersHelp: every page's heading line carries the act
// that shows the help and folds it again, the ? key named as its shortcut,
// with help's one glyph and both its words, and both its words are in the
// Slovenian catalog, as is the row glyph's name.
func TestTheHeadingLineOffersHelp(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{}), "/system/maintenance").Body.String()
	masthead := strings.Index(body, "data-verso-masthead")
	page := strings.Index(body, `class="verso-page__body"`)
	if masthead < 0 || page < masthead {
		t.Fatalf("no masthead before the page body:\n%s", body)
	}
	line := body[masthead:page]
	for _, want := range []string{"data-verso-help-toggle", `aria-keyshortcuts="?"`, ">Show help<", ">Hide help<", `<path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"`} {
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
	for en, sl := range map[string]string{"Show help": "Prikaži pomoč", "Hide help": "Skrij pomoč", "Help": "Pomoč"} {
		if catalog[en] != sl {
			t.Errorf("the Slovenian catalog says %q for %q, want %q", catalog[en], en, sl)
		}
	}
}

// TestTheSignInPageIgnoresHelp: the sign-in page has no help to show and
// says nothing of the cookie.
func TestTheSignInPageIgnoresHelp(t *testing.T) {
	s := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: helpCookie, Value: "shown"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if root := rootTag(rec.Body.String()); root == "" || strings.Contains(root, "data-verso-help") {
		t.Errorf("the sign-in page takes the help: %q", root)
	}
}

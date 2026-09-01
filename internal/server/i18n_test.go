// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// fakeSL is a partial Slovenian catalog: enough to prove both localization seams —
// a nav label wrapped in Go (tr) and a page-chrome literal baked into the template
// ({{ t }}) — while a deliberately-omitted key exercises the per-key English
// fallback.
var fakeSL = fstest.MapFS{
	"sl/base.json": {Data: []byte(`{
		"Home": "Domov",
		"Advanced": "Napredno",
		"Sign in": "Prijava"
	}`)},
}

// getLang issues an authenticated GET carrying an Accept-Language header, so the
// per-request negotiation is exercised the way a real browser drives it. It reads
// in the advanced mode (ADR-015), where the section titles and the plugin nav
// labels are on the page for the localization to reach.
func getLang(t *testing.T, srv *Server, path, acceptLanguage string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	req.AddCookie(&http.Cookie{Name: modeCookie, Value: widget.ModeAdvanced})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}

// withFakeSL installs the partial Slovenian catalog on a fresh server.
func withFakeSL(t *testing.T, srv *Server) {
	t.Helper()
	bundle, problems := i18n.Load(fakeSL, "*/*.json")
	if len(problems) != 0 {
		t.Fatalf("loading fake catalog: %v", problems)
	}
	srv.SetBundle(bundle)
}

// TestAcceptLanguageRendersSlovenian proves the end-to-end path: a request whose
// Accept-Language selects an installed catalog renders <html lang> plus both
// localization seams in that language.
func TestAcceptLanguageRendersSlovenian(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	withFakeSL(t, srv)

	body := getLang(t, srv, "/system/access", "sl-SI,sl;q=0.9,en;q=0.5")
	for _, want := range []string{
		`<html lang="sl">`, // negotiated language on the root element
		">Domov<",          // a nav label localized in Go (tr)
		">Napredno<",       // page chrome localized in the template ({{ t }})
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sl render missing %q", want)
		}
	}
}

// TestNoAcceptLanguageRendersEnglish confirms the default: without an installed
// match the page is English, and the negotiated <html lang> is "en".
func TestNoAcceptLanguageRendersEnglish(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	withFakeSL(t, srv)

	body := getLang(t, srv, "/system/access", "")
	for _, want := range []string{`<html lang="en">`, ">Home<", ">Advanced<"} {
		if !strings.Contains(body, want) {
			t.Errorf("English render missing %q", want)
		}
	}
	if strings.Contains(body, "Domov") || strings.Contains(body, "Napredno") {
		t.Errorf("English render must not contain Slovenian: %s", body)
	}
}

// TestSlovenianFallsBackPerKey pins the graceful-degradation guarantee: a request
// in Slovenian still renders an untranslated key ("Devices", absent from the
// catalog) in English, never blank or a raw key.
func TestSlovenianFallsBackPerKey(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	withFakeSL(t, srv)

	body := getLang(t, srv, "/system/access", "sl")
	if !strings.Contains(body, ">Domov<") {
		t.Errorf("translated key missing: expected Domov")
	}
	if !strings.Contains(body, ">Devices<") {
		t.Errorf("untranslated key must fall back to English source (Devices): %s", body)
	}
}

// TestLoginNegotiatesLanguage proves the pre-session login page localizes too: it
// carries no session, so Accept-Language is its only language signal.
func TestLoginNegotiatesLanguage(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	withFakeSL(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Header.Set("Accept-Language", "sl")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="sl">`) || !strings.Contains(body, "Prijava") {
		t.Errorf("login did not localize from Accept-Language: %s", body)
	}
}

// TestPluginPageAndNavUseTheirCatalog proves the ownership split (ADR-012 §5): a
// plugin's page and nav label are localized from the plugin's OWN catalog, while
// shell-owned widget defaults still come from base — and the plugin's strings never
// leak into the shell's base translator.
func TestPluginPageAndNavUseTheirCatalog(t *testing.T) {
	env := &plugin.Envelope{
		Title:      "Demo",
		Subheading: "A demo page",
		Widget:     json.RawMessage(`{"type":"form","submit":"Save","fields":[{"type":"field","name":"h","label":"Hostname"}]}`),
	}
	m := plugin.Manifest{
		ManifestVersion: 1, ID: "demo", Name: "Demo", Socket: "/demo.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Network", Label: "Widgets", Path: "/"}},
	}
	srv := newServerWith(t, fakeBackend{}, &fakeTransport{env: env}, []plugin.Manifest{m})
	srv.probe = func(string) bool { return true }

	bundle, problems := i18n.Load(fstest.MapFS{
		"sl/base.json": {Data: []byte(`{"Save":"Shrani","Network":"Omrežje","Hostname":"BASE-WRONG"}`)},
		"sl/demo.json": {Data: []byte(`{"Hostname":"Ime gostitelja","Demo":"Predstavitev","A demo page":"Predstavitvena stran","Widgets":"Gradniki"}`)},
	}, "*/*.json")
	if len(problems) != 0 {
		t.Fatalf("catalog problems: %v", problems)
	}
	srv.SetBundle(bundle)

	body := getLang(t, srv, "/plugins/demo/", "sl")
	for _, want := range []string{
		"Ime gostitelja",       // the plugin's field label, from the plugin catalog (wins over base)
		"Predstavitev",         // the plugin page heading (env.Title), from the plugin catalog
		"Predstavitvena stran", // the plugin subheading, from the plugin catalog
		"Gradniki",             // the plugin's nav label, from the plugin catalog
		">Shrani<",             // shell-owned widget default, from base via base⊕plugin fallthrough
		"Omrežje",              // the section title, shell taxonomy, from base
	} {
		if !strings.Contains(body, want) {
			t.Errorf("plugin render missing %q:\n%s", want, body)
		}
	}
	// The plugin's field label must resolve to the plugin's value, not the (wrong)
	// base one — proving the overlay precedence.
	if strings.Contains(body, "BASE-WRONG") {
		t.Error("base catalog leaked over the plugin's own value")
	}
	// The base translator alone must not know the plugin's strings.
	if base := bundle.Translator("sl"); base("Widgets") != "Widgets" {
		t.Errorf("plugin string leaked into the base translator: %q", base("Widgets"))
	}
}

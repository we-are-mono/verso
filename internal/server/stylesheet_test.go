// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builtStylesheetHref is the address the build's stylesheet is linked at: its
// content's hash in the name, so a new build is a new address.
func builtStylesheetHref() string {
	sum := sha256.Sum256([]byte(cssText))
	return "/assets/verso." + hex.EncodeToString(sum[:])[:12] + ".css"
}

// fetchPublic requests path as a browser with no session does.
func fetchPublic(t *testing.T, s *Server, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// TestEveryPageLinksTheStylesheetByItsContent: a page links the build's
// stylesheet in its head rather than carrying it, so the router writes it
// once per browser instead of into every page; the theme stays inline after
// it, where setTheme swaps it in place.
func TestEveryPageLinksTheStylesheetByItsContent(t *testing.T) {
	s := newServer(t, fakeBackend{})
	link := `<link rel="stylesheet" href="` + builtStylesheetHref() + `">`
	for name, body := range map[string]string{
		"a page":         get(t, s, "/devices").Body.String(),
		"the login page": fetchPublic(t, s, "/login", nil).Body.String(),
	} {
		at := strings.Index(body, link)
		if at < 0 || at > strings.Index(body, "</head>") {
			t.Errorf("%s does not link the stylesheet in its head", name)
		}
		if strings.Contains(body, `<style id="verso-css">`) || strings.Contains(body, ".verso-meter{") {
			t.Errorf("%s still carries the stylesheet inline", name)
		}
		if theme := strings.Index(body, `<style id="verso-theme">`); theme < at {
			t.Errorf("%s carries the theme before the stylesheet", name)
		}
	}
	// Every page the shell draws whole (the takeovers too) links it the same way.
	files, err := filepath.Glob(filepath.Join("templates", "*.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(src, []byte("<head>")) {
			continue
		}
		if !bytes.Contains(src, []byte(`<link rel="stylesheet" href="{{.Stylesheet}}">`)) {
			t.Errorf("%s does not link the stylesheet", file)
		}
		if bytes.Contains(src, []byte(".CSS }}")) || bytes.Contains(src, []byte(".CSS}}")) {
			t.Errorf("%s still inlines the stylesheet", file)
		}
	}
}

// TestTheStylesheetIsKeptForever: the stylesheet's address names its content,
// so a browser keeps it for good and never asks again for this build; a new
// build is a new address, so no browser ever shows a stale one. It travels
// compressed to a browser that takes it so.
func TestTheStylesheetIsKeptForever(t *testing.T) {
	s := newServer(t, fakeBackend{})
	plain := fetchPublic(t, s, builtStylesheetHref(), nil)
	if plain.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", builtStylesheetHref(), plain.Code)
	}
	if got := plain.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := plain.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the stylesheet kept for good", got)
	}
	if got := plain.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
	if plain.Body.String() != cssText {
		t.Error("the stylesheet served is not the build's")
	}

	packed := fetchPublic(t, s, builtStylesheetHref(), map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if got := packed.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if packed.Body.Len() >= len(cssText)/4 {
		t.Errorf("the compressed stylesheet is %d bytes of %d", packed.Body.Len(), len(cssText))
	}
	zr, err := gzip.NewReader(packed.Body)
	if err != nil {
		t.Fatal(err)
	}
	unpacked, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(unpacked) != cssText {
		t.Error("the compressed stylesheet is not the build's")
	}

	if stale := fetchPublic(t, s, "/assets/verso.000000000000.css", nil); stale.Code != http.StatusNotFound {
		t.Errorf("another build's stylesheet = %d, want 404", stale.Code)
	}
}

// TestADevSessionLinksTheLiveStylesheet: under scripts/dev.sh a page links
// the stylesheet at its stable address, read fresh from the drop file and
// never kept, so the dev script can swap the link when a block changes.
func TestADevSessionLinksTheLiveStylesheet(t *testing.T) {
	dev := newServer(t, fakeBackend{})
	dev.devCSS = filepath.Join(t.TempDir(), "verso-dev.css")
	if err := os.WriteFile(dev.devCSS, []byte("main{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if body := get(t, dev, "/devices").Body.String(); !strings.Contains(body, `<link rel="stylesheet" href="/assets/verso.css">`) {
		t.Error("a dev page does not link the live stylesheet")
	}
	live := fetchPublic(t, dev, "/assets/verso.css", nil)
	if live.Body.String() != "main{}" {
		t.Errorf("the live stylesheet = %q, want the drop file", live.Body.String())
	}
	if got := live.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("the live stylesheet is kept: Cache-Control = %q", got)
	}
}

// TestStylesComeFromTheShell: the policy lets a page take its stylesheet from
// the router and its own style attributes (the theme, a meter's share), and
// nothing from anywhere else.
func TestStylesComeFromTheShell(t *testing.T) {
	csp := get(t, newServer(t, fakeBackend{}), "/").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'self' 'unsafe-inline';") {
		t.Errorf("CSP style-src = %q, want 'self' 'unsafe-inline'", csp)
	}
}

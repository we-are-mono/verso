// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var iconHref = regexp.MustCompile(`<link rel="(?:icon|apple-touch-icon|mask-icon|manifest)" href="([^"]+)"`)

// Every full page the shell renders names the same icons in its head, so the
// tab, the bookmark and the home-screen icon are the same mark on every page,
// the sign-in page and the takeover pages included.
func TestEveryPageHeadNamesTheIcons(t *testing.T) {
	err := fs.WalkDir(templateFS, "templates", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		src, _ := templateFS.ReadFile(path)
		if strings.Contains(string(src), "<head>") && !strings.Contains(string(src), `{{template "verso-icons"}}`) {
			t.Errorf("%s has a <head> without the icons", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Each icon the head names is served, publicly (a browser asks for it before
// anyone signs in), with the type its file is.
func TestTheNamedIconsAreServed(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	login := httptest.NewRecorder()
	srv.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))

	hrefs := iconHref.FindAllStringSubmatch(login.Body.String(), -1)
	if len(hrefs) != 7 {
		t.Fatalf("login head names %d icon links, want 7", len(hrefs))
	}
	types := map[string]string{
		".ico": "image/", ".svg": "image/svg+xml", ".png": "image/png",
		".webmanifest": "application/manifest+json",
	}
	for _, m := range hrefs {
		href := m[1]
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, href, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", href, rec.Code)
			continue
		}
		for ext, want := range types {
			if strings.HasSuffix(href, ext) && !strings.HasPrefix(rec.Header().Get("Content-Type"), want) {
				t.Errorf("GET %s Content-Type = %q, want %s…", href, rec.Header().Get("Content-Type"), want)
			}
		}
	}
}

// A browser that reads no head (or asks before it has one) looks for the
// icon at the site root.
func TestFaviconAtTheRootIsPublic(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.ico = %d, want 200", rec.Code)
	}
	want, _ := scriptFS.ReadFile("assets/icons/favicon.ico")
	if rec.Body.String() != string(want) {
		t.Errorf("GET /favicon.ico is not the embedded icon")
	}
}

// The manifest's icons are files the shell serves: a stale path leaves an
// installed app with a blank icon and no error anywhere.
func TestManifestIconsAreServed(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	src, err := scriptFS.ReadFile("assets/icons/site.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Icons []struct{ Src string } `json:"icons"`
	}
	if err := json.Unmarshal(src, &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if len(manifest.Icons) == 0 {
		t.Fatal("manifest names no icons")
	}
	for _, icon := range manifest.Icons {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, icon.Src, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("manifest icon %s = %d, want 200", icon.Src, rec.Code)
		}
	}
}

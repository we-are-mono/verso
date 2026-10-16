// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestAnAddressWithNothingAtItIsAPage: a page someone navigated to that is not
// there — a typo, a stale bookmark, a plugin that is gone — is answered inside
// the shell with a heading that says so and the way home, never the server's
// bare line. A fragment, a stream or a script's fetch gets the bare 404 it can
// read.
func TestAnAddressWithNothingAtItIsAPage(t *testing.T) {
	s := newServer(t, fakeBackend{access: true})
	for _, path := range []string{"/nope", "/system/nope/deeper", "/plugins/nope/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := serve(t, s, req)
		body := rec.Body.String()
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
		for _, want := range []string{`verso-page-heading`, ">Page not found<", "There is nothing at this address", `href="/"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the page is missing %q:\n%s", path, want, body)
			}
		}
	}
	// A plugin that has nothing at a path of its own says so in its envelope,
	// and the page is the shell's, as for any address with nothing at it.
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Status: http.StatusOK, NotFound: true,
		Widget: json.RawMessage(`{"type":"text","markdown":"There is nothing here."}`)}}
	ps := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	req := httptest.NewRequest(http.MethodGet, "/plugins/demo/nope", nil)
	req.Header.Set("Accept", "text/html")
	if rec := serve(t, ps, req); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), ">Page not found<") {
		t.Errorf("a plugin's own not-found: %d\n%s", rec.Code, rec.Body.String())
	}

	fetch := httptest.NewRequest(http.MethodGet, "/nope", nil)
	fetch.Header.Set("HX-Request", "true")
	fetch.Header.Set("Accept", "text/html")
	if rec := serve(t, s, fetch); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "verso-page-heading") {
		t.Errorf("a fragment's 404 is the bare line: %d %q", rec.Code, rec.Body.String())
	}
}

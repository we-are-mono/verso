// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

func overviewTestManifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "jodu-5174x", Name: "5G <ODU>", Socket: "/run/jodu.sock", SchemaVersion: 1,
		Overview: &plugin.OverviewEntry{Path: "/overview", Icon: "radio"},
		ACL:      plugin.ACL{Read: []plugin.ACLScope{{Scope: "uci", Object: "jodu5174x", Function: "read"}}},
	}
}

func TestHomepageRendersPluginOverviewFromScopedSnapshot(t *testing.T) {
	reads := 0
	var readSIDs []string
	backend := fakeBackend{uciReads: &reads, uciReadSIDs: &readSIDs, uci: map[string]map[string]any{
		"jodu5174x": {"main": map[string]any{".type": "jodu5174x", "host": "private-value"}},
	}}
	transport := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"properties","items":[{"label":"Device <tag>","value":"<img src=x onerror=1>"}]}`),
	}}
	s := newServerWith(t, backend, transport, []plugin.Manifest{overviewTestManifest()})
	rec := get(t, s, "/")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("homepage status = %d", rec.Code)
	}
	id := widget.PluginOverviewCardID("jodu-5174x")
	for _, want := range []string{`data-overview-card="` + id + `"`, `>5G &lt;ODU&gt;</h2>`, `href="/plugins/jodu-5174x/"`, `Device &lt;tag&gt;`, `&lt;img src=x onerror=1&gt;`} {
		if !strings.Contains(body, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
	if strings.Contains(body, "private-value") || strings.Contains(body, "<img src=x") {
		t.Fatal("plugin markup or UCI snapshot escaped the widget renderer")
	}
	if transport.lastSocket != "/run/jodu.sock" || transport.lastReq.Method != http.MethodGet || transport.lastReq.Path != "/overview" || transport.lastReq.Form != nil || !transport.hasDeadline {
		t.Fatalf("plugin request: socket=%q request=%+v", transport.lastSocket, transport.lastReq)
	}
	if got := transport.lastReq.UCI["jodu5174x"]["main"].(map[string]any)["host"]; got != "private-value" || reads == 0 {
		t.Fatalf("snapshot=%v reads=%d", transport.lastReq.UCI, reads)
	}
	if len(readSIDs) == 0 || !slices.Contains(readSIDs, "test-sid") {
		t.Fatalf("UCI reads used sessions %v, want operator session", readSIDs)
	}
}

func TestHomepageDoesNotFetchHiddenPluginOverview(t *testing.T) {
	transport := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Status: http.StatusOK, Widget: json.RawMessage(`{"type":"properties","items":[]}`)}}
	id := widget.PluginOverviewCardID("jodu-5174x")
	layout := widget.DefaultOverviewLayoutWithPlugins([]string{"jodu-5174x"})
	layout.Cols[0] = slices.DeleteFunc(layout.Cols[0], func(cardID string) bool { return cardID == id })
	layout.Hidden = append(layout.Hidden, id)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: layout.Encode()})
	rec := serve(t, newServerWith(t, fakeBackend{}, transport, []plugin.Manifest{overviewTestManifest()}), req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || transport.lastReq.Method != "" {
		t.Fatalf("hidden card fetched: status=%d request=%+v", rec.Code, transport.lastReq)
	}
	if !strings.Contains(body, `data-overview-restore="`+id+`"`) || strings.Contains(body, `data-overview-card="`+id+`"`) {
		t.Fatal("hidden plugin card did not remain on the Arrange shelf")
	}
}

func TestHomepageContainsUnavailableOrUnsafePluginBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		tr   *fakeTransport
	}{
		{name: "transport failure", tr: &fakeTransport{err: errors.New("offline")}},
		{name: "write envelope", tr: &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Status: 200, Commit: []plugin.CommitOp{{Config: "jodu5174x"}}, Widget: json.RawMessage(`{"type":"properties","items":[]}`)}}},
		{name: "not found envelope", tr: &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Status: 200, NotFound: true, Widget: json.RawMessage(`{"type":"properties","items":[]}`)}}},
		{name: "form body", tr: &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Status: 200, Widget: json.RawMessage(`{"type":"form","style":"page"}`)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, newServerWith(t, fakeBackend{}, tc.tr, []plugin.Manifest{overviewTestManifest()}), "/")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "This plugin’s Overview reading is unavailable.") {
				t.Fatalf("failed card broke homepage: status=%d", rec.Code)
			}
		})
	}
}

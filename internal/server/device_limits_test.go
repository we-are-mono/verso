// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
)

type limitSummaryTransport struct {
	*tabTransport
	summaries   *[]plugin.EntitySummary
	reads       int
	clearOnSave bool
}

func (t *limitSummaryTransport) Fetch(ctx context.Context, socket string, req plugin.Request) (*plugin.Envelope, error) {
	if req.Path == "/entity/device/" {
		t.reads++
		return &plugin.Envelope{SchemaVersion: 1, Entities: t.summaries}, nil
	}
	if req.Method == http.MethodPost && t.clearOnSave {
		empty := []plugin.EntitySummary{}
		t.summaries = &empty
	}
	return t.tabTransport.Fetch(ctx, socket, req)
}

func limitRosterServer(t *testing.T, tr *limitSummaryTransport) *Server {
	t.Helper()
	m := shapingManifest()
	m.Nav = nil
	m.EntityTabs[0].Summaries = true
	backend := rosterBackend()
	backend.access = true
	s := newServerWith(t, backend, tr, []plugin.Manifest{m})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	return s
}

func TestDeviceLimitsStayFindableWithoutRuntimePresence(t *testing.T) {
	entries := []plugin.EntitySummary{
		{ID: "42:E6:AD:FF:B7:AF", State: "Internet schedule", Details: []plugin.EntitySummaryDetail{{Label: "Hours (router time)", Values: []string{"21:00–07:00"}}, {Label: "Download", Values: []string{"25 Mbit/s"}}}},
		{ID: "02:11:22:33:44:55", Name: "Old tablet", State: "Internet block"},
		{ID: "02:11:22:33:44:55", Name: "Duplicate", State: "Speed limit"},
		{ID: "invalid", Name: "Invalid device", State: "Speed limit"},
	}
	tr := &limitSummaryTransport{tabTransport: twoTabs(), summaries: &entries}
	s := limitRosterServer(t, tr)
	body := get(t, s, "/devices").Body.String()
	for _, want := range []string{"With limits", "Hours (router time): 21:00–07:00", "Download: 25 Mbit/s", "Internet block", "Old tablet", `limited`, `data-verso-entity-url="/entity/device/02:11:22:33:44:55"`} {
		if !strings.Contains(body, want) {
			t.Errorf("roster missing %q", want)
		}
	}
	for _, absent := range []string{"Duplicate", "Invalid device", `href="/plugins/qos/"`, `title="Block internet"`} {
		if strings.Contains(body, absent) {
			t.Errorf("roster should omit %q", absent)
		}
	}
	if tr.reads != 1 {
		t.Fatalf("roster read summaries %d times; want one bulk read", tr.reads)
	}
	// Even after every lease and neighbor expires, the saved device opens.
	s.readLeases = func() ([]byte, error) { return nil, nil }
	s.neighbors = func() ([]sysstat.Neighbor, error) { return nil, nil }
	rec := get(t, s, "/entity/device/02:11:22:33:44:55?tab=shape")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Old tablet") || !strings.Contains(rec.Body.String(), `hx-post="/entity/device/02:11:22:33:44:55?tab=shape"`) {
		t.Fatalf("offline device's editor is unreachable: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, s, "/plugins/qos/"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/devices" {
		t.Errorf("old landing page should lead to Devices: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDeviceLimitsUnavailableIsNotNoLimits(t *testing.T) {
	tr := &limitSummaryTransport{tabTransport: twoTabs()}
	body := get(t, limitRosterServer(t, tr), "/devices").Body.String()
	if !strings.Contains(body, "Some offline devices may be missing") || strings.Contains(body, "With limits") {
		t.Fatal("an unavailable read must show a notice, not a zero-count limits filter")
	}
	entries := []plugin.EntitySummary{}
	tr.summaries = &entries
	body = get(t, limitRosterServer(t, tr), "/devices").Body.String()
	if strings.Contains(body, "Some offline devices may be missing") || !strings.Contains(body, "With limits") {
		t.Fatal("a successful empty read must keep the limits filter without a failure notice")
	}
}

func TestRemovingLastOfflineLimitKeepsSuccessfulSaveVisible(t *testing.T) {
	entries := []plugin.EntitySummary{{ID: "02:11:22:33:44:55", Name: "Old tablet", State: "Internet block"}}
	tr := &limitSummaryTransport{tabTransport: twoTabs(), summaries: &entries, clearOnSave: true}
	tr.bySocket["/run/verso/qos.sock"].Commit = []plugin.CommitOp{{Config: "firewall", Section: "old_limit", Delete: true}}
	s := limitRosterServer(t, tr)
	rec := postPlugin(t, s, "/entity/device/02:11:22:33:44:55?tab=shape", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Old tablet") {
		t.Fatalf("removing the final limit lost the successful editor response: %d", rec.Code)
	}
	if rec.Header().Get("HX-Trigger") != "verso-entity-saved" {
		t.Error("successful save should refresh the roster on close")
	}
	if strings.Contains(get(t, s, "/devices").Body.String(), "Old tablet") {
		t.Error("offline device without limits should leave the roster")
	}
}

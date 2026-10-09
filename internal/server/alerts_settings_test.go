// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/history"
)

func withAlerter(t *testing.T, s *Server) *history.Store {
	t.Helper()
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	s.alerter = alerts.New(store, "home-router")
	return store
}

// TestAlertsConfigReadsVersoAlerts: absent kinds are the defaults, and an
// address absent sends nothing.
func TestAlertsConfigReadsVersoAlerts(t *testing.T) {
	got := alertsConfig(map[string]any{"alerts": map[string]any{
		".type": "alerts", "url": "https://ntfy.sh/home", "kind": []any{"boot"},
	}})
	if got.URL != "https://ntfy.sh/home" || got.Heartbeat != "" || !slices.Equal(got.Kinds, []string{"boot"}) {
		t.Fatalf("config = %+v", got)
	}
	if got := alertsConfig(map[string]any{}); got.URL != "" || got.Kinds != nil {
		t.Fatalf("no section = %+v, want nothing sent and default kinds", got)
	}
}

// TestAlertsFormShowsWhatIsSet: the Events page offers Alerts on its heading
// line, and the form holds the addresses and the kinds as they stand.
func TestAlertsFormShowsWhatIsSet(t *testing.T) {
	s := newServer(t, fakeBackend{uci: map[string]map[string]any{"verso": {"alerts": map[string]any{
		".type": "alerts", "url": "https://ntfy.sh/home", "heartbeat": "https://hc-ping.com/abc",
	}}}})
	withAlerter(t, s)
	s.alerter.Configure(alerts.Config{URL: "https://ntfy.sh/home"})
	if _, page := getSignedIn(t, s, eventsPath); !strings.Contains(page, `href="/system/events/alerts"`) {
		t.Fatal("the Events page does not offer Alerts")
	}
	status, body := getSignedIn(t, s, alertsPath)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	for _, want := range []string{
		`name="url"`, `value="https://ntfy.sh/home"`, `name="heartbeat"`, `value="https://hc-ping.com/abc"`,
		`value="wan" checked`, `value="boot" checked`, `value="rollback" checked`, `value="storage" checked`, "Send a test alert",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("alerts form missing %q", want)
		}
	}
}

// TestSavingAlertsStagesThem: addresses stage into verso.alerts; an emptied
// one is removed, absent meaning off; the default kinds leave no list behind,
// and none chosen is said as "none".
func TestSavingAlertsStagesThem(t *testing.T) {
	var writes []uciWrite
	var deletes []string
	s := newServer(t, fakeBackend{writes: &writes, deletes: &deletes, uci: map[string]map[string]any{"verso": {"alerts": map[string]any{
		".type": "alerts", "heartbeat": "https://hc-ping.com/old", "kind": []any{"wan"},
	}}}})
	withAlerter(t, s)
	rec := postPlugin(t, s, alertsPath, url.Values{"url": {"https://ntfy.sh/home"}, "heartbeat": {""}, "kind": {"wan", "boot", "rollback", "storage"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body:\n%s", rec.Code, rec.Body)
	}
	if len(writes) != 1 || writes[0].config != "verso" || writes[0].section != "alerts" || writes[0].values["url"] != "https://ntfy.sh/home" {
		t.Fatalf("writes = %+v", writes)
	}
	if !slices.Contains(deletes, "verso.alerts.heartbeat") || !slices.Contains(deletes, "verso.alerts.kind") {
		t.Fatalf("deletes = %v, want the empty heartbeat and the default kinds removed", deletes)
	}

	writes, deletes = nil, nil
	postPlugin(t, s, alertsPath, url.Values{"url": {"https://ntfy.sh/home"}})
	if kinds, _ := writes[0].values["kind"].([]string); !slices.Equal(kinds, []string{"none"}) {
		t.Fatalf("no kind chosen wrote %v, want [none]", writes[0].values["kind"])
	}
}

// TestAConfirmedApplyReachesTheRunningAlerter: procd keeps the new command
// line for the next start, so the confirm itself hands the alerter its
// settings.
func TestAConfirmedApplyReachesTheRunningAlerter(t *testing.T) {
	s := newServer(t, fakeBackend{uci: map[string]map[string]any{"verso": {"alerts": map[string]any{
		".type": "alerts", "url": "https://ntfy.sh/home",
	}}}})
	withAlerter(t, s)
	if s.alerter.Configured() {
		t.Fatal("the alerter had an address before the apply")
	}
	if rec := postPlugin(t, s, "/uci/confirm", nil); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	if !s.alerter.Configured() {
		t.Fatal("the confirmed address did not reach the alerter")
	}
}

// TestSavingAlertsRefusesWhatIsNotAWebAddress.
func TestSavingAlertsRefusesWhatIsNotAWebAddress(t *testing.T) {
	var writes []uciWrite
	s := newServer(t, fakeBackend{writes: &writes, uci: map[string]map[string]any{"verso": {}}})
	withAlerter(t, s)
	for _, bad := range []string{"ntfy.sh/home", "ftp://example.com/x", "https://"} {
		rec := postPlugin(t, s, alertsPath, url.Values{"url": {bad}})
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Enter a web address starting with https:// or http://") {
			t.Errorf("%q answered %d", bad, rec.Code)
		}
	}
	if len(writes) != 0 {
		t.Fatalf("a refused address was staged: %+v", writes)
	}
}

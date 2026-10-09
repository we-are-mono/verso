// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// applyServer is a server with one change staged on the demo plugin's page
// and the rollback window's timer in the test's hand.
func applyServer(t *testing.T) (*Server, *history.Store, *func()) {
	t.Helper()
	confirms := 0
	s := newServerWith(t, fakeBackend{confirms: &confirms, changes: map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	}}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	store := withAlerter(t, s)
	var window func()
	s.afterRollback = func(_ time.Duration, f func()) { window = f }
	return s, store, &window
}

func recorded(t *testing.T, store *history.Store) []history.Event {
	t.Helper()
	events, _, err := store.EventsAfter(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// TestAConfirmedApplyIsRecorded: changes kept are recorded with the pages
// they were made on, and the window passing afterwards records nothing more.
func TestAConfirmedApplyIsRecorded(t *testing.T) {
	s, store, window := applyServer(t)
	if rec := postPlugin(t, s, "/uci/apply", nil); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d", rec.Code)
	}
	if got := recorded(t, store); len(got) != 0 {
		t.Fatalf("an apply not yet confirmed is recorded: %+v", got)
	}
	if rec := postPlugin(t, s, "/uci/confirm", nil); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	(*window)()
	got := recorded(t, store)
	if len(got) != 1 || got[0].Kind != "changes.applied" || got[0].Detail != `{"pages":["Demo Plugin"]}` {
		t.Fatalf("events = %+v, want one applied on the plugin's page", got)
	}
}

// TestAnApplyNobodyConfirmedIsRecordedAsRolledBack: the router reverted it,
// because the browser could not reach it to confirm.
func TestAnApplyNobodyConfirmedIsRecordedAsRolledBack(t *testing.T) {
	s, store, window := applyServer(t)
	postPlugin(t, s, "/uci/apply", nil)
	if *window == nil {
		t.Fatal("the apply armed no window")
	}
	(*window)()
	got := recorded(t, store)
	if len(got) != 1 || got[0].Kind != "changes.rolledback" || got[0].Detail != `{"pages":["Demo Plugin"]}` {
		t.Fatalf("events = %+v, want one rolled back on the plugin's page", got)
	}
}

// TestAppliedAndRolledBackChangesReadAsSentences.
func TestAppliedAndRolledBackChangesReadAsSentences(t *testing.T) {
	identity := func(s string) string { return s }
	applied := eventRow(history.Event{Kind: "changes.applied", Detail: `{"pages":["Firewall","Interfaces"]}`}, time.Now(), identity)
	if got := applied.Cells[2]; got.Text != "Changes applied: Firewall, Interfaces" || got.Variant != "neutral" {
		t.Errorf("applied = %+v", got)
	}
	back := eventRow(history.Event{Kind: "changes.rolledback", Detail: `{"pages":["Firewall"]}`}, time.Now(), identity)
	if got := back.Cells[2]; got.Text != "Changes rolled back: Firewall. The router could not be reached to confirm them." || got.Variant != "warning" {
		t.Errorf("rolled back = %+v", got)
	}
}

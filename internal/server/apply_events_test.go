// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"reflect"
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

// changeLog keeps what the shell hands config history (ADR-021).
type changeLog struct{ changes []history.Change }

func (l *changeLog) RecordChange(_ context.Context, c history.Change) (int64, error) {
	l.changes = append(l.changes, c)
	return int64(len(l.changes)), nil
}

// changeServer is applyServer with the stage given and config history kept
// in a changeLog.
func changeServer(t *testing.T, stage map[string][][]string, manifests ...plugin.Manifest) (*Server, *changeLog, *func()) {
	t.Helper()
	confirms := 0
	if len(manifests) == 0 {
		manifests = []plugin.Manifest{demoACLManifest()}
	}
	s := newServerWith(t, fakeBackend{confirms: &confirms, changes: stage}, &fakeTransport{}, manifests)
	withAlerter(t, s)
	log := &changeLog{}
	s.changeLog = log
	var window func()
	s.afterRollback = func(_ time.Duration, f func()) { window = f }
	return s, log, &window
}

func text(s string) *string { return &s }

// TestAConfirmedApplyIsKeptAsAChange: config history gets the apply as the
// review drawer read it, who applied it from where, and each uci change under
// its row.
func TestAConfirmedApplyIsKeptAsAChange(t *testing.T) {
	s, log, window := changeServer(t, map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	})
	postPlugin(t, s, "/uci/apply", nil)
	if len(log.changes) != 0 {
		t.Fatalf("an apply not yet confirmed is kept: %+v", log.changes)
	}
	postPlugin(t, s, "/uci/confirm", nil)
	(*window)()

	if len(log.changes) != 1 {
		t.Fatalf("kept %d changes, want 1", len(log.changes))
	}
	got := log.changes[0]
	if got.Status != history.ChangeApplied || got.User != "root" || got.Address == "" || got.Time.IsZero() {
		t.Fatalf("change = %+v", got)
	}
	want := []history.ChangeItem{{
		Page: "Demo Plugin", Plugin: "demo", Verb: "changed", Words: "system: @system[0].hostname = pending", Verbatim: true,
		Ops: []history.ChangeOp{{Config: "system", Section: "@system[0]", Op: "set", Option: "hostname", Value: text("pending")}},
	}}
	if !reflect.DeepEqual(got.Items, want) {
		t.Fatalf("items\n%+v\nwant\n%+v", got.Items, want)
	}
}

// TestAnApplyNobodyConfirmedIsKeptAsRolledBack: the change the router undid
// is kept, as rolled back, to be staged again.
func TestAnApplyNobodyConfirmedIsKeptAsRolledBack(t *testing.T) {
	s, log, window := changeServer(t, map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	})
	postPlugin(t, s, "/uci/apply", nil)
	(*window)()
	if len(log.changes) != 1 || log.changes[0].Status != history.ChangeRolledBack {
		t.Fatalf("changes = %+v, want one rolled back", log.changes)
	}
}

// TestASecretOptionIsMarkedSecret: an option OpenWrt keeps a secret in, and
// one a plugin's manifest names, reach config history marked, so neither
// value is stored.
func TestASecretOptionIsMarkedSecret(t *testing.T) {
	m := demoACLManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "network", Function: "write"})
	m.Secrets = []string{"system.token"}
	s, log, window := changeServer(t, map[string][][]string{
		"network": {{"set", "wan", "password", "hunter22"}, {"set", "wan", "username", "t2-41877"}},
		"system":  {{"set", "@system[0]", "token", "abc"}},
	}, m)
	postPlugin(t, s, "/uci/apply", nil)
	postPlugin(t, s, "/uci/confirm", nil)
	(*window)()

	secret := map[string]bool{}
	for _, item := range log.changes[0].Items {
		for _, op := range item.Ops {
			secret[op.Config+"."+op.Option] = op.Secret
		}
	}
	if !secret["network.password"] || !secret["system.token"] || secret["network.username"] {
		t.Fatalf("secret = %v, want network.password and system.token only", secret)
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

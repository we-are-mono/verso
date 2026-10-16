// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// applyServer is a server with one change staged on the demo plugin's page,
// history.db keeping its events and config history, and the rollback
// window's timer in the test's hand.
func applyServer(t *testing.T) (*Server, *history.Store, *func()) {
	t.Helper()
	confirms := 0
	s := newServerWith(t, fakeBackend{confirms: &confirms, changes: map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	}}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	store := withAlerter(t, s)
	s.changeLog = store
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
	if len(got) != 1 || got[0].Kind != "changes.applied" || got[0].Detail != `{"pages":["Demo Plugin"],"change":1}` {
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
	if len(got) != 1 || got[0].Kind != "changes.rolledback" || got[0].Detail != `{"pages":["Demo Plugin"],"change":1}` {
		t.Fatalf("events = %+v, want one rolled back on the plugin's page", got)
	}
}

// changeLog keeps what the shell hands config history (ADR-021): each change,
// and which entries each undid.
type changeLog struct {
	changes []history.Change
	undone  map[int64][]int64 // by → the entries it undid
}

func (l *changeLog) RecordChange(_ context.Context, c history.Change) (int64, error) {
	l.changes = append(l.changes, c)
	return int64(len(l.changes)), nil
}

func (l *changeLog) MarkUndone(_ context.Context, ids []int64, by int64) error {
	if l.undone == nil {
		l.undone = map[int64][]int64{}
	}
	l.undone[by] = append(l.undone[by], ids...)
	return nil
}

// changeServer is applyServer with the stage given and config history kept
// in a changeLog.
func changeServer(t *testing.T, stage map[string][][]string, manifests ...plugin.Manifest) (*Server, *changeLog, *func()) {
	t.Helper()
	confirms := 0
	return keptServer(t, fakeBackend{confirms: &confirms, changes: stage}, &fakeTransport{}, manifests...)
}

// keptServer is a server on backend and tr with config history kept in a
// changeLog and the rollback window's timer in the test's hand.
func keptServer(t *testing.T, backend fakeBackend, tr *fakeTransport, manifests ...plugin.Manifest) (*Server, *changeLog, *func()) {
	t.Helper()
	if len(manifests) == 0 {
		manifests = []plugin.Manifest{demoACLManifest()}
	}
	s := newServerWith(t, backend, tr, manifests)
	withAlerter(t, s)
	log := &changeLog{}
	s.changeLog = log
	var window func()
	s.afterRollback = func(_ time.Duration, f func()) { window = f }
	return s, log, &window
}

// applyConfirmed applies the stage, confirms it, and lets the window close.
func applyConfirmed(t *testing.T, s *Server, window *func()) {
	t.Helper()
	postPlugin(t, s, "/uci/apply", nil)
	postPlugin(t, s, "/uci/confirm", nil)
	(*window)()
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

// TestAnApplyKeepsWhatEachChangeReplaced: each uci change carries what the
// router held before it, read committed before the apply: an option's value,
// a list whole, a removed section whole; a new section replaced nothing.
func TestAnApplyKeepsWhatEachChangeReplaced(t *testing.T) {
	m := demoACLManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "network", Function: "write"})
	confirms := 0
	s, log, window := keptServer(t, fakeBackend{confirms: &confirms,
		changes: map[string][][]string{
			"system": {{"set", "main", "hostname", "new"}},
			"network": {
				{"list-add", "lan", "ports", "lan3"},
				{"remove", "lan", "ip6assign"},
				{"remove", "wan6"},
				{"add", "guest", "interface"},
			},
		},
		committed: map[string]map[string]any{
			"system": {"main": map[string]any{".type": "system", "hostname": "old"}},
			"network": {
				"lan":  map[string]any{".type": "interface", "ports": []any{"lan1", "lan2"}, "ip6assign": "60"},
				"wan6": map[string]any{".type": "interface", "device": "@wan", "proto": "dhcpv6"},
			},
		},
	}, &fakeTransport{}, m)
	applyConfirmed(t, s, window)

	old := map[string]string{}
	for _, item := range log.changes[0].Items {
		for _, op := range item.Ops {
			old[op.Op+" "+op.Config+"."+op.Section+"."+op.Option] = string(op.Old)
		}
	}
	want := map[string]string{
		"set system.main.hostname":            `"old"`,
		"list-add network.lan.ports":          `["lan1","lan2"]`,
		"remove-option network.lan.ip6assign": `"60"`,
		"remove-section network.wan6.":        `{".type":"interface","device":"@wan","proto":"dhcpv6"}`,
		"add-section network.guest.interface": ``,
	}
	if !log.changes[0].OldRead {
		t.Error("the change should say what it replaced was read")
	}
	if !reflect.DeepEqual(old, want) {
		t.Fatalf("old values\n%v\nwant\n%v", old, want)
	}
}

// TestAnApplyIsKeptWhenWhatItReplacedCannotBeRead: an apply still goes into
// config history when the committed read fails, without old values.
func TestAnApplyIsKeptWhenWhatItReplacedCannotBeRead(t *testing.T) {
	confirms := 0
	s, log, window := keptServer(t, fakeBackend{confirms: &confirms,
		changes:      map[string][][]string{"system": {{"set", "main", "hostname", "new"}}},
		committedErr: errors.New("helper unavailable"),
	}, &fakeTransport{})
	applyConfirmed(t, s, window)

	if len(log.changes) != 1 || log.changes[0].Items[0].Ops[0].Old != nil || log.changes[0].OldRead {
		t.Fatalf("changes = %+v, want one without an old value", log.changes)
	}
}

// TestEachRowKeepsWhatItIsAbout: a row keeps the subject its plugin named,
// a subject with no name is called by its section, and a kind outside the
// vocabulary leaves the row to be named by its config and section, as a row
// no plugin described is.
func TestEachRowKeepsWhatItIsAbout(t *testing.T) {
	confirms := 0
	s, log, window := keptServer(t, fakeBackend{confirms: &confirms, changes: map[string][][]string{
		"system": {
			{"set", "main", "hostname", "edge"},
			{"set", "cfg0792bd", "enabled", "0"},
			{"set", "w1", "size", "2"},
			{"set", "main", "log_size", "64"},
		},
	}}, &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Router name: edge", Covers: []int{0}, Subject: &plugin.Subject{Kind: "setting", Name: "Router name"}},
		{Plain: "Turned off the rule.", Covers: []int{1}, Subject: &plugin.Subject{Kind: "rule"}},
		{Plain: "Changed the widget.", Covers: []int{2}, Subject: &plugin.Subject{Kind: "widget", Name: "w"}},
	}})
	applyConfirmed(t, s, window)

	got := map[string][2]string{}
	for _, item := range log.changes[0].Items {
		got[item.Words] = [2]string{item.SubjectKind, item.SubjectName}
	}
	want := map[string][2]string{
		"Router name: edge":          {"setting", "Router name"},
		"Turned off the rule.":       {"rule", "cfg0792bd"},
		"Changed the widget.":        {"", ""},
		"system: main.log_size = 64": {"", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subjects\n%v\nwant\n%v", got, want)
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

// TestAnApplyKeepsItsNote: the review drawer's note goes into config history
// with the apply, trimmed and kept to a line.
func TestAnApplyKeepsItsNote(t *testing.T) {
	for _, tc := range []struct{ typed, kept string }{
		{"  Ping off for the audit \n", "Ping off for the audit"},
		{"", ""},
		{strings.Repeat("é", 300), strings.Repeat("é", applyNoteLimit)},
	} {
		s, log, window := changeServer(t, map[string][][]string{"system": {{"set", "@system[0]", "hostname", "pending"}}})
		postPlugin(t, s, "/uci/apply", url.Values{"note": {tc.typed}})
		postPlugin(t, s, "/uci/confirm", nil)
		(*window)()
		if got := log.changes[0].Note; got != tc.kept {
			t.Errorf("note %q kept as %q, want %q", tc.typed, got, tc.kept)
		}
	}
}

// TestThePendingPageAsksForANote: an optional box in the form Apply posts,
// saying what it is for and where it is kept; its hint says only that it is
// optional, never words that read as a note already written.
func TestThePendingPageAsksForANote(t *testing.T) {
	s, _, _ := changeServer(t, map[string][][]string{"system": {{"set", "@system[0]", "hostname", "pending"}}})
	body := get(t, s, pendingPath).Body.String()
	mustContain(t, body, `name="note"`, `placeholder="Optional"`, `id="verso-pending-apply"`,
		"Say why you’re making these changes. It’s kept with them in <strong>System › Changes</strong>.")
	if strings.Contains(body, `placeholder="system.@system[0] changed"`) {
		t.Error("the note's hint reads as a note already written")
	}
}

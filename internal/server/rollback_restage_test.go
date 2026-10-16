// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

func wirelessManifest() plugin.Manifest {
	m := dhcpManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "wireless", Function: "write"})
	return m
}

// rolledBackServer applies a stage of dhcp and wireless changes, the guest
// network's key among them, with the rollback window's timer in the test's
// hand.
func rolledBackServer(t *testing.T) (*Server, stageFake, *func()) {
	t.Helper()
	f := newStageFake(map[string][][]string{
		"dhcp":     {{"set", "lan", "leasetime", "6h"}},
		"wireless": {{"set", "guest", "key", "tidepool-gate-7"}},
	}, nil)
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{wirelessManifest()})
	var window func()
	s.afterRollback = func(_ time.Duration, fn func()) { window = fn }
	return s, f, &window
}

// TestARolledBackApplyKeepsItsChanges: the router put its settings back
// because the page never checked in; the changes the apply carried are
// pending again as they were, a secret's value with them, each with the time
// it was first staged, and the page is told when it happened.
func TestARolledBackApplyKeepsItsChanges(t *testing.T) {
	s, f, window := rolledBackServer(t)
	staged := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	s.authors.note("test-sid", optionAddress("dhcp", "lan", "leasetime"), stageAuthor{Plugin: "dnsdhcp", Page: "DHCP", Href: "/plugins/dnsdhcp/", At: staged})

	if rec := postPlugin(t, s, "/uci/apply", nil); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d", rec.Code)
	}
	if len(f.j.uci) != 0 {
		t.Fatalf("the apply left %v staged", f.j.uci)
	}
	(*window)()

	want := map[string][][]string{
		"dhcp":     {{"set", "lan", "leasetime", "6h"}},
		"wireless": {{"set", "guest", "key", "tidepool-gate-7"}},
	}
	if !reflect.DeepEqual(f.j.uci, want) {
		t.Errorf("after the rollback the stage holds %v, want %v", f.j.uci, want)
	}
	if who, ok := s.authors.of("test-sid", optionAddress("dhcp", "lan", "leasetime")); !ok || !who.At.Equal(staged) {
		t.Errorf("the change's record = %+v, want its first time", who)
	}
	end, ok := s.stageEnds.of("test-sid")
	if !ok || end.Kind != stageRolledBack || end.At.IsZero() {
		t.Errorf("the page is told %+v, want the rollback and when", end)
	}
}

// TestAKeptApplyStagesNothingAgain: an apply the page confirmed is kept, and
// the window passing afterwards stages nothing.
func TestAKeptApplyStagesNothingAgain(t *testing.T) {
	s, f, window := rolledBackServer(t)
	postPlugin(t, s, "/uci/apply", nil)
	if rec := postPlugin(t, s, "/uci/confirm", nil); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	(*window)()
	if len(f.j.uci) != 0 {
		t.Errorf("a kept apply staged %v again", f.j.uci)
	}
	if end, ok := s.stageEnds.of("test-sid"); !ok || end.Kind != stageApplied || end.Count != 2 {
		t.Errorf("the page is told %+v, want two changes applied", end)
	}
}

// TestAnExpiredSessionsRollbackStagesNothing: a session rpcd no longer knows
// cannot be staged into; the rollback is still told, and nothing panics.
func TestAnExpiredSessionsRollbackStagesNothing(t *testing.T) {
	s, f, window := rolledBackServer(t)
	postPlugin(t, s, "/uci/apply", nil)
	f.j.denied = true
	(*window)()
	if len(f.j.uci) != 0 {
		t.Errorf("an expired session's stage holds %v", f.j.uci)
	}
	if end, ok := s.stageEnds.of("test-sid"); !ok || end.Kind != stageRolledBack {
		t.Errorf("the page is told %+v, want the rollback", end)
	}
}

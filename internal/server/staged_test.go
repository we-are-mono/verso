// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestPluginSaveStagesWithoutCommit: a plugin's commit intent stages through
// rpcd and stops there (ADR-010) — the capsule owns apply; the broker must
// never commit.
func TestPluginSaveStagesWithoutCommit(t *testing.T) {
	writes := []uciWrite{}
	commits := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &writes, commits: &commits}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(writes) != 1 {
		t.Fatalf("UCISet calls = %d, want 1 (the write must stage)", len(writes))
	}
	if len(commits) != 0 {
		t.Fatalf("UCICommit calls = %v, want none — only the capsule commits (ADR-010)", commits)
	}
}

// TestCapsuleRendersPendingChanges: the capsule is server-rendered from UCI's
// stage — visible when changes are pending, scoped to configs plugins declare,
// with the change tuples humanized in the review list.
func TestCapsuleRendersPendingChanges(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system":  {{"set", "@system[0]", "hostname", "verso-lab"}},
			"network": {{"set", "lan", "ipaddr", "10.0.0.2"}}, // undeclared: not the shell's to show
		},
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/").Body.String()
	for _, want := range []string{
		"1 staged change",                          // the undeclared config is filtered out
		"system: @system[0].hostname = verso-lab", // mechanical humanization
		"verso-capsule-review",                    // the review affordance appears with changes
	} {
		if !strings.Contains(body, want) {
			t.Errorf("capsule missing %q", want)
		}
	}
	if strings.Contains(body, `id="verso-capsule-apply" disabled`) {
		t.Error("a dirty page must not disable Save & Apply")
	}
}

// TestCapsuleInertWhenClean: the bar is always in the flow — clean pages show it
// with the actions disabled and no review list.
func TestCapsuleInertWhenClean(t *testing.T) {
	s := newServer(t, fakeBackend{})
	body := get(t, s, "/").Body.String()
	for _, want := range []string{
		"No pending changes",
		`id="verso-capsule-apply" disabled`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("clean bar missing %q", want)
		}
	}
	if strings.Contains(body, "verso-capsule-review") {
		t.Error("a clean page has nothing to review")
	}
}

// TestUCIApplyRoute: Apply calls rpcd's uci apply with the rollback window.
func TestUCIApplyRoute(t *testing.T) {
	applies := []int{}
	s := newServer(t, fakeBackend{applies: &applies})
	rec := postPlugin(t, s, "/uci/apply", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(applies) != 1 || applies[0] != uciRollbackTimeout {
		t.Errorf("UCIApply calls = %v, want one with the %ds rollback window", applies, uciRollbackTimeout)
	}
}

// TestUCIConfirmRoute: Confirm disarms the pending rollback.
func TestUCIConfirmRoute(t *testing.T) {
	confirms := 0
	s := newServer(t, fakeBackend{confirms: &confirms})
	rec := postPlugin(t, s, "/uci/confirm", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if confirms != 1 {
		t.Errorf("UCIConfirm calls = %d, want 1", confirms)
	}
}

// TestUCIDiscardRevertsDeclaredConfigs: Discard reverts every dirty config a
// plugin declares — and only those; a stage on an undeclared config is not the
// shell's to manage.
func TestUCIDiscardRevertsDeclaredConfigs(t *testing.T) {
	reverts := []string{}
	s := newServerWith(t, fakeBackend{
		changes: map[string][][]string{
			"system":  {{"set", "@system[0]", "hostname", "x"}},
			"network": {{"set", "lan", "ipaddr", "y"}},
		},
		reverts: &reverts,
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/uci/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(reverts) != 1 || reverts[0] != "system" {
		t.Errorf("reverts = %v, want [system] only", reverts)
	}
}

// TestUCIApplyRequiresCSRF: the capsule's posts sit behind the same CSRF gate as
// every state-changing request (VS-04).
func TestUCIApplyRequiresCSRF(t *testing.T) {
	s := newServer(t, fakeBackend{})
	rec := postForm(t, s, "/uci/apply", url.Values{})
	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d; an unauthenticated, tokenless apply must not succeed", rec.Code)
	}
}

// TestHumanizeChange: the mechanical rendering of uci change tuples.
func TestHumanizeChange(t *testing.T) {
	cases := []struct {
		ch   []string
		want string
	}{
		{[]string{"set", "lan", "ipaddr", "10.0.0.2"}, "network: lan.ipaddr = 10.0.0.2"},
		{[]string{"set", "guest", "zone"}, "network: new zone section guest"},
		{[]string{"add", "cfg0b", "rule"}, "network: new rule section cfg0b"},
		{[]string{"remove", "lan", "ipaddr"}, "network: remove lan.ipaddr"},
		{[]string{"remove", "lan"}, "network: remove lan"},
		{[]string{"list-add", "ntp", "server", "a.pool"}, "network: ntp.server += a.pool"},
		{[]string{"rename", "lan", "trusted"}, "network: rename lan trusted"},
	}
	for _, c := range cases {
		if got := humanizeChange("network", c.ch); got != c.want {
			t.Errorf("humanizeChange(%v) = %q, want %q", c.ch, got, c.want)
		}
	}
}

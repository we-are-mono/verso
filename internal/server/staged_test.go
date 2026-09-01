// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
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
		"1 pending change",                        // the undeclared config is filtered out
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

// TestCapsuleInertWhenClean: staging pages (plugin pages) keep the bar in the
// flow even when clean — actions disabled, no review list. Immediate-action
// pages (the Overview) drop the bar entirely when nothing is staged.
func TestCapsuleInertWhenClean(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	body := get(t, s, "/plugins/demo/").Body.String()
	for _, want := range []string{
		"No pending changes",
		`id="verso-capsule-apply" disabled`,
		"verso-capsule mr-8 mb-10 rounded-r-sm border-y border-r border-slate-200 bg-surface-subtle", // right edge aligns to content; left stays flush
		"dark:border-gray-700",
		"dark:bg-sky-700 dark:text-gray-100 dark:hover:bg-sky-800 dark:active:bg-sky-900",
		"dark:text-gray-300 dark:hover:bg-gray-900 dark:active:bg-gray-950",
		`class="ml-auto inline-flex items-center`, // status balances actions on the right
	} {
		if !strings.Contains(body, want) {
			t.Errorf("clean staging page missing %q", want)
		}
	}
	if strings.Contains(body, `id="verso-capsule-review"`) {
		t.Error("a clean page has nothing to review")
	}
	if strings.Contains(body, "fixed inset-x-0 bottom-0") {
		t.Error("the staged-changes footer must follow content, not pin to the viewport")
	}

	if home := get(t, s, "/").Body.String(); strings.Contains(home, `id="verso-capsule"`) {
		t.Error("a clean immediate-action page must not carry the bar")
	}
}

// TestImmediatePluginPageOmitsCleanCapsule: direct-command pages do not imply
// a second Save & Apply step. The flag changes chrome only; existing shared
// staged changes are still shown by renderPage's capsule count.
func TestImmediatePluginPageOmitsCleanCapsule(t *testing.T) {
	env := &plugin.Envelope{
		SchemaVersion: 1, Title: "Access", Immediate: true, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}
	tr := &fakeTransport{env: env}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	if body := get(t, s, "/plugins/demo/").Body.String(); strings.Contains(body, `id="verso-capsule"`) {
		t.Error("a clean immediate-command plugin page must not carry the capsule")
	}

	dirty := newServerWith(t, fakeBackend{changes: map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	}}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})
	if body := get(t, dirty, "/plugins/demo/").Body.String(); !strings.Contains(body, "1 pending change") {
		t.Error("an immediate-command page must still reveal pending changes from elsewhere")
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

func TestPluginApplyActionRunsAfterUCIApply(t *testing.T) {
	var sequence []string
	manifest := demoACLManifest()
	manifest.ACL.Write = append(manifest.ACL.Write, plugin.ACLScope{
		Scope: "ubus", Object: "verso", Function: "setSystemTime",
	})
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "router"}}},
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "2026-08-30T12:34:56", "timezone": "CET-1CEST,M3.5.0,M10.5.0/3",
		}}},
	}}
	s := newServerWith(t, fakeBackend{
		access:  true,
		applies: &[]int{},
		setSystemTime: func(_ context.Context, sid, datetime, timezone string) error {
			sequence = append(sequence, "time "+sid+" "+datetime+" "+timezone)
			return nil
		},
	}, tr, []plugin.Manifest{manifest})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"router"}}); rec.Code != http.StatusOK {
		t.Fatalf("prepare status = %d, want 200", rec.Code)
	}
	if rec := postPlugin(t, s, "/uci/apply", nil); rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200", rec.Code)
	}
	if len(sequence) != 1 || sequence[0] != "time test-sid 2026-08-30T12:34:56 CET-1CEST,M3.5.0,M10.5.0/3" {
		t.Fatalf("apply action calls = %v", sequence)
	}
	if len(s.takePendingApply("test-sid")) != 0 {
		t.Fatal("successful apply must clear its one-shot action")
	}
}

// TestPendingApplyIsSessionScoped: one session's armed apply tail is invisible to
// another session's Save & Apply, an empty save never wipes it, and draining
// clears it atomically.
func TestPendingApplyIsSessionScoped(t *testing.T) {
	s := newServer(t, fakeBackend{})
	action := plugin.ApplyAction{Name: "set-system-time", Args: map[string]string{
		"datetime": "2026-08-30T12:34:56", "timezone": "GMT0",
	}}
	s.setPendingApply("alice", []plugin.ApplyAction{action})

	// Bob's Save & Apply drains only Bob's (empty) tail — never Alice's.
	if got := s.takePendingApply("bob"); len(got) != 0 {
		t.Fatalf("bob's apply drained %d actions from alice's tail", len(got))
	}
	s.setPendingApply("alice", nil) // an unrelated save with no tail must not wipe it
	// Alice's own apply drains exactly her one action, then clears it atomically.
	if got := s.takePendingApply("alice"); len(got) != 1 {
		t.Fatalf("alice drained %d actions, want 1 (survived bob + empty save)", len(got))
	}
	if got := s.takePendingApply("alice"); len(got) != 0 {
		t.Fatal("a drained tail must be cleared")
	}
}

func TestPluginApplyActionRequiresDeclaredScope(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "2026-08-30T12:34:56", "timezone": "GMT0",
		}}},
	}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoACLManifest()})
	if rec := postPlugin(t, s, "/plugins/demo/", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestNestedAlternateFieldErrorBlocksTransaction(t *testing.T) {
	called := false
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"form","style":"page","fields":[
			{"type":"section","children":[{"type":"conditional","name":"enabled","checked":false,
			"otherwise":[{"type":"stack","children":[{"type":"field","name":"datetime","error":"Invalid time"}]}]}]}
		]}`),
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "bad", "timezone": "GMT0",
		}}},
	}}
	manifest := demoACLManifest()
	manifest.ACL.Write = append(manifest.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "setSystemTime"})
	s := newServerWith(t, fakeBackend{
		access:        true,
		setSystemTime: func(context.Context, string, string, string) error { called = true; return nil },
	}, tr, []plugin.Manifest{manifest})
	if rec := postPlugin(t, s, "/plugins/demo/", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called || len(s.takePendingApply("test-sid")) != 0 {
		t.Fatal("invalid alternate field must not prepare or execute an apply action")
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

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

// refusal is a helper refusal carrying the words the operator reads.
type refusal string

func (r refusal) Error() string             { return "refused: " + string(r) }
func (r refusal) ValidationMessage() string { return string(r) }

// actBackend records the registered acts it runs, answering with err.
type actBackend struct {
	fakeBackend
	ran *[]string
	err error
}

func (b actBackend) PluginCommand(_ context.Context, _, plugin, name string, args map[string]string) (json.RawMessage, error) {
	encoded, _ := json.Marshal(args)
	*b.ran = append(*b.ran, plugin+"."+name+" "+string(encoded))
	return json.RawMessage(`{}`), b.err
}

// postAct posts a form whose answer names commands, from a plugin declaring
// write, and returns the answer and what the backend ran.
func postAct(t *testing.T, write []plugin.ACLScope, commands []plugin.ApplyAction, commit []plugin.CommitOp, err error) (*http.Response, string, []string) {
	t.Helper()
	var ran []string
	m := demoManifest()
	m.ACL.Write = write
	env := &plugin.Envelope{SchemaVersion: 1, Title: "Dynamic DNS", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"ddns"}`), Commands: commands, Commit: commit}
	s := newServerWith(t, actBackend{fakeBackend: fakeBackend{access: true}, ran: &ran, err: err}, &fakeTransport{env: env}, []plugin.Manifest{m})
	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"update": {"home"}})
	return rec.Result(), rec.Body.String(), ran
}

var declaredUpdate = []plugin.ACLScope{{Scope: "verso-plugin", Object: "ddns", Function: "update"}}

// TestARegisteredActRunsAsDeclared: an act named <plugin>.<name> runs through
// the helper with its arguments when the plugin declares it, and is refused
// without asking the helper when it does not.
func TestARegisteredActRunsAsDeclared(t *testing.T) {
	act := []plugin.ApplyAction{{Name: "ddns.update", Args: map[string]string{"section": "home"}}}
	res, _, ran := postAct(t, declaredUpdate, act, nil, nil)
	if res.StatusCode >= 400 || len(ran) != 1 || ran[0] != `ddns.update {"section":"home"}` {
		t.Fatalf("a declared act: status %d, ran %v", res.StatusCode, ran)
	}
	res, _, ran = postAct(t, nil, act, nil, nil)
	if res.StatusCode < 400 || len(ran) != 0 {
		t.Errorf("an undeclared act: status %d, ran %v", res.StatusCode, ran)
	}
	other := []plugin.ApplyAction{{Name: "vpn.restart"}}
	if res, _, ran := postAct(t, declaredUpdate, other, nil, nil); res.StatusCode < 400 || len(ran) != 0 {
		t.Errorf("an act of a scope not declared: status %d, ran %v", res.StatusCode, ran)
	}
}

// TestARefusedActSaysItsWords: the root half's refusal reaches the page's
// notice as written.
func TestARefusedActSaysItsWords(t *testing.T) {
	act := []plugin.ApplyAction{{Name: "ddns.update", Args: map[string]string{"section": "home"}}}
	res, body, _ := postAct(t, declaredUpdate, act, nil, refusal("That section does not exist."))
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "That section does not exist.") {
		t.Errorf("status %d, body without the refusal", res.StatusCode)
	}
}

// TestAnActNeverSharesASave: an act beside a uci commit is refused as not
// permitted, and neither runs.
func TestAnActNeverSharesASave(t *testing.T) {
	act := []plugin.ApplyAction{{Name: "ddns.update", Args: map[string]string{"section": "home"}}}
	commit := []plugin.CommitOp{{Config: "ddns", Section: "home", Values: map[string]any{"enabled": "1"}}}
	write := append([]plugin.ACLScope{{Scope: "uci", Object: "ddns", Function: "write"}}, declaredUpdate...)
	res, _, ran := postAct(t, write, act, commit, nil)
	if res.StatusCode != http.StatusForbidden || len(ran) != 0 {
		t.Errorf("an act beside a commit: status %d, ran %v", res.StatusCode, ran)
	}
}

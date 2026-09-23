// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
)

// accessKeysServer is Access with the system plugin's part composed into it:
// the plugin answers with its set of keys, a live reading of what was typed,
// and — when it was asked to add one — the command that adds it.
func accessKeysServer(t *testing.T, b *credentialsFake, commands []plugin.ApplyAction) *Server {
	t.Helper()
	b.access = true // the operator's session may write what the plugin declares
	m := credentialManifest()
	m.Socket, m.SystemAccess, m.Name = "/run/system.sock", "/access", "System"
	m.SchemaVersion = supportedSchemaVersion
	widget := `{"type":"collection","items":[],"empty":"No keys are authorized.",
	  "add":{"label":"Add a key","name":"authorized_key","submit":"Add key",
	    "preview":{"type":"code","value":"256 SHA256:x me (ED25519)","live":true}}}`
	env := &plugin.Envelope{SchemaVersion: 1, Title: "Access", Widget: json.RawMessage(widget), Commands: commands}
	if len(commands) > 0 {
		env.Notice = &plugin.Notice{Level: "success", Text: "Key added."}
	}
	return newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
}

// TestAccessAnswersAKeyReadingAlone: while a key is typed, the page asks the
// plugin what it reads and gets the reading alone — the page around the box is
// already on screen — and nothing is added for asking.
func TestAccessAnswersAKeyReadingAlone(t *testing.T) {
	b := &credentialsFake{data: openwrt.AccessCredentials{AuthorizedKeys: ""}}
	key := testPublicKey(t)
	s := accessKeysServer(t, b, []plugin.ApplyAction{{Name: "ssh-key-add", Args: map[string]string{"key": key}}})
	rec := postPluginAs(t, s, "/system/access?plugin=system", url.Values{"authorized_key": {key}}, "preview")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "data-verso-preview") || strings.Contains(body, "<html") {
		t.Fatalf("want the reading alone, got %d:\n%s", rec.Code, body)
	}
	if b.written != "" {
		t.Errorf("asking what a key reads as added it: %q", b.written)
	}
}

// TestACommandThatFailsIsSaidOnce: when the router refuses an act, the page
// says so once, in its notice — not again inside every form on the page, none
// of which was what failed.
func TestACommandThatFailsIsSaidOnce(t *testing.T) {
	b := &credentialsFake{data: openwrt.AccessCredentials{AuthorizedKeys: ""}, conflict: true}
	b.access = true
	m := credentialManifest()
	m.Socket, m.SystemAccess, m.Name = "/run/system.sock", "/access", "System"
	m.SchemaVersion = supportedSchemaVersion
	widget := `{"type":"stack","children":[
	  {"type":"form","style":"settings","submit":"Save","fields":[{"type":"field","name":"Port","label":"Port"}]},
	  {"type":"form","style":"settings","submit":"Save","fields":[{"type":"field","name":"listen","label":"Listen"}]},
	  {"type":"collection","items":[],"empty":"No keys are authorized."}]}`
	env := &plugin.Envelope{SchemaVersion: 1, Title: "Access", Widget: json.RawMessage(widget),
		Notice:   &plugin.Notice{Level: "success", Text: "Key added."},
		Commands: []plugin.ApplyAction{{Name: "ssh-key-add", Args: map[string]string{"key": testPublicKey(t)}}}}
	s := newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
	rec := postPlugin(t, s, "/system/access?plugin=system", url.Values{"authorized_key": {testPublicKey(t)}})
	body := rec.Body.String()
	const message = "The router could not complete the action. Try again."
	if n := strings.Count(body, message); n != 1 {
		t.Errorf("the failure is said %d times, want once:\n%s", n, body)
	}
	if strings.Contains(body, "Key added.") {
		t.Error("a failed act must not also say it succeeded")
	}
}

// TestAccessRereadsAfterAKeyIsAdded: a key added in place is written at once,
// and the page is read again from the router, so the new key stands in the set
// with what happened said once.
func TestAccessRereadsAfterAKeyIsAdded(t *testing.T) {
	b := &credentialsFake{data: openwrt.AccessCredentials{AuthorizedKeys: ""}}
	key := testPublicKey(t)
	s := accessKeysServer(t, b, []plugin.ApplyAction{{Name: "ssh-key-add", Args: map[string]string{"key": key}}})
	rec := postPlugin(t, s, "/system/access?plugin=system", url.Values{"authorized_key": {key}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/access" {
		t.Fatalf("want a fresh read of Access, got %d → %q", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(b.written, key) {
		t.Errorf("the key was not written: %q", b.written)
	}
}

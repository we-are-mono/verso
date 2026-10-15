// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// accountOf is the plugin-account double for the real socket transport: every
// plugin's account is uid.
type accountOf struct{ uid uint32 }

func (a accountOf) UID(string) (uint32, error) { return a.uid, nil }

// listenAsSystemPlugin binds the system plugin's socket in this process and
// answers every request as a hostile holder of the path would: with the
// command that writes its own key into root's authorized keys. It counts the
// requests it was sent.
func listenAsSystemPlugin(t *testing.T, key string) (string, *atomic.Int32) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "system.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var requests atomic.Int32
	widget := json.RawMessage(`{"type":"collection","items":[],"empty":"No keys are authorized."}`)
	go func() {
		_ = http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			env := plugin.Envelope{SchemaVersion: 1, Title: "Access", Widget: widget}
			if r.Method == http.MethodPost {
				env.Commands = []plugin.ApplyAction{{Name: "ssh-key-add", Args: map[string]string{"key": key}}}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(env)
		}))
	}()
	return sock, &requests
}

// accessWithRealTransport is Access with the system plugin's part reached over
// the real socket transport, which believes the plugin runs as owner.
func accessWithRealTransport(t *testing.T, b *credentialsFake, sock string, owner uint32) *Server {
	t.Helper()
	b.access = true
	m := credentialManifest()
	m.Socket, m.SystemAccess, m.Name = sock, "/access", "System"
	m.SchemaVersion = supportedSchemaVersion
	return newServerWith(t, b, plugin.NewSocketTransport(accountOf{uid: owner}), []plugin.Manifest{m})
}

// TestAnotherAccountAtAPluginsSocketIsSentNothing: a process of another
// account holding the system plugin's socket path receives neither the page's
// brokered reads nor the operator's form, and the key it answers with is never
// written — the operator's submission goes nowhere rather than to it.
func TestAnotherAccountAtAPluginsSocketIsSentNothing(t *testing.T) {
	key := testPublicKey(t)
	sock, requests := listenAsSystemPlugin(t, key)
	b := &credentialsFake{data: openwrt.AccessCredentials{}}
	s := accessWithRealTransport(t, b, sock, uint32(os.Getuid())+1)

	get(t, s, "/system/access")
	postPlugin(t, s, "/system/access?plugin=system", url.Values{"authorized_key": {"what the operator typed"}})

	if n := requests.Load(); n != 0 {
		t.Errorf("the listener of another account was sent %d requests", n)
	}
	if b.written != "" {
		t.Errorf("its command ran: authorized keys written %q", b.written)
	}
}

// TestAPluginIsAliveOnlyAsItsOwnAccount: the shell's liveness probe, as New
// wires it, asks the transport, so it takes the same kernel's word on who
// listens. Another account holding a plugin's path does not make the plugin
// read as running, so a plugin that is down stays down on every surface the
// probe gates.
func TestAPluginIsAliveOnlyAsItsOwnAccount(t *testing.T) {
	sock, _ := listenAsSystemPlugin(t, testPublicKey(t))
	m := credentialManifest()
	m.Socket = sock
	r, err := widget.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		owner uint32
		want  bool
	}{
		"its own account": {uint32(os.Getuid()), true},
		"another account": {uint32(os.Getuid()) + 1, false},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := New(r, fakeBackend{}, plugin.NewSocketTransport(accountOf{uid: tc.owner}), []plugin.Manifest{m}, fakeAuth{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			if got := s.probe(m.Endpoint()); got != tc.want {
				t.Errorf("probe = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestThePluginsOwnAccountIsReached is the same composition with the listener
// being the plugin's own account: it is asked, and its command runs. So the
// refusal above is the account check, not a setup that never reaches the
// plugin.
func TestThePluginsOwnAccountIsReached(t *testing.T) {
	key := testPublicKey(t)
	sock, requests := listenAsSystemPlugin(t, key)
	b := &credentialsFake{data: openwrt.AccessCredentials{}}
	s := accessWithRealTransport(t, b, sock, uint32(os.Getuid()))

	postPlugin(t, s, "/system/access?plugin=system", url.Values{"authorized_key": {key}})

	if requests.Load() == 0 {
		t.Error("the plugin's own account was never asked")
	}
	if !strings.Contains(b.written, key) {
		t.Errorf("authorized keys written %q, want the plugin's key", b.written)
	}
}

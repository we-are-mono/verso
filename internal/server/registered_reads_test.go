// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/tlscert"
)

// registryBackend answers registered root commands (ADR-027) from a table
// keyed "<plugin>.<name>", after an optional delay, and records each call.
type registryBackend struct {
	fakeBackend
	answers map[string]string
	delay   time.Duration
	mu      *sync.Mutex
	calls   *[]string
}

func (b registryBackend) PluginCommand(_ context.Context, sid, plugin, name string, args map[string]string) (json.RawMessage, error) {
	time.Sleep(b.delay)
	key := plugin + "." + name
	b.mu.Lock()
	*b.calls = append(*b.calls, key)
	b.mu.Unlock()
	answer, ok := b.answers[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return json.RawMessage(answer), nil
}

func newRegistryBackend(answers map[string]string) registryBackend {
	return registryBackend{answers: answers, mu: &sync.Mutex{}, calls: &[]string{}}
}

func registryOf(reads ...plugin.RegisteredCommand) *plugin.Registry {
	return &plugin.Registry{Commands: reads}
}

func read(name string, version int) plugin.RegisteredCommand {
	return plugin.RegisteredCommand{Name: name, Kind: "read", Version: version}
}

func pluginRead(object, function string) plugin.ACLScope {
	return plugin.ACLScope{Scope: "verso-plugin", Object: object, Function: function}
}

// TestRegisteredReadsArriveVersioned: a declared read runs through the helper
// and reaches the plugin under <plugin>.<name> as its version and its result,
// another plugin's read included; a read that fails, or whose plugin is not
// installed, gives no entry, and the page still renders.
func TestRegisteredReadsArriveVersioned(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Interfaces", Widget: json.RawMessage(`{"type":"card"}`)}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		pluginRead("vpn", "state"),
		pluginRead("dnsdhcp", "dhcp-state"),
		pluginRead("ddns", "state"),
		pluginRead("vpn", "missing"),
	}}
	vpn := plugin.Manifest{ID: "vpn", Commands: registryOf(read("state", 2))}
	dnsdhcp := plugin.Manifest{ID: "dnsdhcp", Commands: registryOf(read("dhcp-state", 1))}
	backend := newRegistryBackend(map[string]string{"vpn.state": `{"tunnels":[]}`})
	s := newServerWith(t, backend, tr, []plugin.Manifest{m, vpn, dnsdhcp})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := string(tr.lastReq.Ubus["vpn.state"]); got != `{"version":2,"result":{"tunnels":[]}}` {
		t.Errorf("vpn.state = %s", got)
	}
	for _, absent := range []string{"dnsdhcp.dhcp-state", "ddns.state", "vpn.missing"} {
		if _, ok := tr.lastReq.Ubus[absent]; ok {
			t.Errorf("%s: a failed or unregistered read gives no entry", absent)
		}
	}
	// ddns is not installed and vpn registers no "missing": neither is asked.
	for _, call := range *backend.calls {
		if call == "ddns.state" || call == "vpn.missing" {
			t.Errorf("asked the helper for %s", call)
		}
	}
}

// TestARegisteredCertificateArrivesAsFacts: a field the registry names as a
// certificate reaches the plugin as the facts the certificate card shows, never
// as PEM; one that does not parse is left out and the rest of the answer kept.
func TestARegisteredCertificateArrivesAsFacts(t *testing.T) {
	certPEM, _, err := tlscert.Generate([]string{"router.example.com"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	answer, _ := json.Marshal(map[string]any{"domain": "router.example.com", "certificate": string(certPEM), "spare": "not a certificate"})
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Demo", Widget: json.RawMessage(`{"type":"card"}`)}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{pluginRead("acme", "state")}}
	acme := plugin.Manifest{ID: "acme", Commands: &plugin.Registry{
		Commands:     []plugin.RegisteredCommand{read("state", 1)},
		Certificates: map[string][]string{"state": {"certificate", "spare"}},
	}}
	s := newServerWith(t, newRegistryBackend(map[string]string{"acme.state": string(answer)}), tr, []plugin.Manifest{m, acme})

	get(t, s, "/plugins/demo/")
	raw := string(tr.lastReq.Ubus["acme.state"])
	if strings.Contains(raw, "BEGIN CERTIFICATE") || strings.Contains(raw, "not a certificate") {
		t.Fatalf("PEM or an unreadable field passed through: %s", raw)
	}
	var got struct {
		Result struct {
			Domain      string            `json:"domain"`
			Certificate map[string]string `json:"certificate"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.Result.Domain != "router.example.com" || got.Result.Certificate["subject"] != "router.example.com" || got.Result.Certificate["fingerprint"] == "" {
		t.Errorf("acme.state = %s", raw)
	}
}

// TestReadsRunTogether: a page's reads run in parallel, so it waits for the
// slowest rather than their sum.
func TestReadsRunTogether(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Demo", Widget: json.RawMessage(`{"type":"card"}`)}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{pluginRead("a", "state"), pluginRead("b", "state"), pluginRead("c", "state")}}
	backend := newRegistryBackend(map[string]string{"a.state": `{}`, "b.state": `{}`, "c.state": `{}`})
	backend.delay = 200 * time.Millisecond
	manifests := []plugin.Manifest{m}
	for _, id := range []string{"a", "b", "c"} {
		manifests = append(manifests, plugin.Manifest{ID: id, Commands: registryOf(read("state", 1))})
	}
	s := newServerWith(t, backend, tr, manifests)

	started := time.Now()
	get(t, s, "/plugins/demo/")
	if elapsed := time.Since(started); elapsed >= 400*time.Millisecond {
		t.Errorf("three 200 ms reads took %v", elapsed)
	}
	if len(tr.lastReq.Ubus) != 3 {
		t.Errorf("reads = %d, want 3", len(tr.lastReq.Ubus))
	}
}

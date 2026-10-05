// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// vpnBackend is a backend that can read what the router's tunnels are doing.
type vpnBackend struct {
	fakeBackend
	state json.RawMessage
}

func (v vpnBackend) VPNState(context.Context, string) (json.RawMessage, error) {
	return v.state, nil
}

// TestPluginVPNStateBrokered: a VPN page that declares the tunnels read
// receives each instance's profile and state and every tunnel's counters in
// its request, verbatim, read through the helper with the operator's sid.
func TestPluginVPNStateBrokered(t *testing.T) {
	state := json.RawMessage(`{"instances":{"proton":{"running":true,"state":"connected","device":"tun0"}},"tunnels":[{"device":"tun0","kind":"openvpn","rx":10,"tx":2}]}`)
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "VPN", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "uci", Object: "openvpn", Function: "read"},
		{Scope: "ubus", Object: "verso", Function: "vpnState"},
	}}
	s := newServerWith(t, vpnBackend{state: state}, tr, []plugin.Manifest{m})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := tr.lastReq.Ubus["vpnState"]; string(got) != string(state) {
		t.Errorf("brokered VPN state = %s, want the backend's JSON verbatim", got)
	}
}

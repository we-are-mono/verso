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

// wirelessBackend is a backend that can read what the radios are doing.
type wirelessBackend struct {
	fakeBackend
	state json.RawMessage
}

func (w wirelessBackend) WirelessState(context.Context, string) (json.RawMessage, error) {
	return w.state, nil
}

// TestPluginWirelessStateBrokered: a Wi-Fi page that declares the wireless
// read receives the radios' live state in its request, read with the
// operator's sid, verbatim — the plugin puts clients and airtime on its rows
// without reaching the radios itself.
func TestPluginWirelessStateBrokered(t *testing.T) {
	state := json.RawMessage(`{"radios":{"radio0":{"up":true,"busy":61}},"networks":{"default_radio0":{"up":true,"clients":3}}}`)
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Wireless", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "uci", Object: "wireless", Function: "read"},
		{Scope: "ubus", Object: "verso", Function: "wirelessState"},
	}}
	s := newServerWith(t, wirelessBackend{state: state}, tr, []plugin.Manifest{m})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := tr.lastReq.Ubus["wirelessState"]; string(got) != string(state) {
		t.Errorf("brokered wireless state = %s, want the backend's JSON verbatim", got)
	}

	// A backend with no way to read the radios gives the page nothing, and the
	// page still renders.
	tr.lastReq = plugin.Request{}
	s = newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{m})
	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without the read", rec.Code)
	}
	if _, ok := tr.lastReq.Ubus["wirelessState"]; ok {
		t.Errorf("a backend that cannot read the radios still reported them: %+v", tr.lastReq.Ubus)
	}
}

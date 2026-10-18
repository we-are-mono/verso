// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestHistoryKeepsAStagedFileAsItsFamilyListsIt: config history records an
// applied file's text as the helper listed it, which its family's transform
// folded (ADR-027), never the file as it lies on disk: an OpenVPN profile's
// key block reaches the history as its placeholder, and no line of it does.
func TestHistoryKeepsAStagedFileAsItsFamilyListsIt(t *testing.T) {
	folded := "client\nremote vpn.example.com 1194\n<tls-crypt> … 3 lines … </tls-crypt>\n"
	f := newStageFake(nil, map[string][][]string{"openvpn": {{"file", "/etc/openvpn/proton.ovpn", folded}}})
	vpn := demoManifest()
	vpn.ID, vpn.Name = "vpn", "VPN"
	vpn.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "openvpn", Function: "write"}}}
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{vpn})

	ctx := context.Background()
	change := s.stagedChangeOf(ctx, "test-sid", s.stagedPlain(ctx, "test-sid"), "root", "192.168.1.10")
	kept, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kept), "… 3 lines …") {
		t.Fatalf("the file's listed text is not in the history: %s", kept)
	}
	for _, key := range []string{"-----BEGIN OpenVPN Static key", "SECRET"} {
		if strings.Contains(string(kept), key) {
			t.Errorf("the history holds %q: %s", key, kept)
		}
	}
}

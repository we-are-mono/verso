// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"testing"
)

func TestFirewallStatusRead(t *testing.T) {
	for _, tc := range []struct {
		name, reply  string
		want         FirewallStatus
		fail, denied bool
	}{
		{"active", `{"status":0,"result":{"state":"active","rules":53}}`, FirewallStatus{State: "active", Rules: 53}, false, false},
		{"inactive", `{"status":0,"result":{"state":"inactive","rules":0}}`, FirewallStatus{State: "inactive"}, false, false},
		{"partial", `{"status":0,"result":{"state":"partial","rules":2}}`, FirewallStatus{State: "partial", Rules: 2}, false, false},
		{"denied", `{"status":6,"error":"permission denied"}`, FirewallStatus{}, true, true},
		{"read failed", `{"status":9,"error":"nft unavailable"}`, FirewallStatus{}, true, false},
		{"no result", `{"status":0}`, FirewallStatus{}, true, false},
		{"no count", `{"status":0,"result":{"state":"active"}}`, FirewallStatus{}, true, false},
		{"negative count", `{"status":0,"result":{"state":"active","rules":-1}}`, FirewallStatus{}, true, false},
		{"unknown state", `{"status":0,"result":{"state":"bogus","rules":2}}`, FirewallStatus{}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socket := helperReplying(t, "firewallStatus", tc.reply)
			backend := &NativeBackend{fwStatus: dialFirewallStatus(socket)}
			got, err := backend.FirewallStatus(context.Background(), "good-sid")
			if (err != nil) != tc.fail || errors.Is(err, ErrAccessDenied) != tc.denied || got != tc.want {
				t.Fatalf("status=%+v, err=%v", got, err)
			}
		})
	}
}

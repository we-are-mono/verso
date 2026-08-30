// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

const testLeases = "1787825263 42:e6:ad:ff:b7:af 192.168.77.102 toms-iphone 01:42:e6:ad:ff:b7:af\n" +
	"1787825264 a2:8c:d7:4a:0e:57 192.168.77.120 * *\n" +
	"1787825265 06:11:22:33:44:55 192.168.77.130 old-printer *\n"

// testNeighbors: .102 is confirmed (reachable) and carries v6 entries on the
// same MAC, .120 has a lapsed entry (stale), .130 has none at all.
func testNeighbors() ([]sysstat.Neighbor, error) {
	return []sysstat.Neighbor{
		{Addr: "192.168.77.102", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x02},        // reachable
		{Addr: "fd42:7ea:aa00:0:1::66", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04}, // its v6, stale
		{Addr: "fe80::44:11", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04},           // its link-local
		{Addr: "192.168.77.120", MAC: "a2:8c:d7:4a:0e:57", Interface: "br-lan", State: 0x04},        // stale
		{Addr: "fe80::dead", MAC: "", Interface: "br-lan", State: 0x20},                             // failed, unresolved
	}, nil
}

// rosterBackend carries the config the zone chip reads: the lan interface's
// subnet, and the firewall zone that covers it.
func rosterBackend() fakeBackend {
	return fakeBackend{uci: map[string]map[string]any{
		"network": {
			"lan": map[string]any{".type": "interface", ".name": "lan",
				"device": "br-lan", "ipaddr": "192.168.77.1", "netmask": "255.255.255.0"},
		},
		"firewall": {
			"z1": map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}},
		},
	}}
}

func rosterServer(t *testing.T) *Server {
	t.Helper()
	s := newServer(t, rosterBackend())
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	s.bridgePorts = func() (map[string]string, error) {
		return map[string]string{"42:e6:ad:ff:b7:af": "lan0"}, nil
	}
	return s
}

func TestConnectedDevicesUseKernelInterfaceWithUCIFallback(t *testing.T) {
	devices := rosterServer(t).connectedDevices(context.Background(), "test-sid")
	byName := map[string]widget.OverviewDevice{}
	for _, device := range devices {
		byName[device.Name] = device
	}
	if got := byName["toms-iphone"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("kernel-backed device = %+v", got)
	}
	if got := byName["old-printer"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("lease-only UCI fallback = %+v", got)
	}
}

// TestLeaseIn: lease expiry reads the way a person says it.
func TestLeaseIn(t *testing.T) {
	now := time.Unix(1000, 0)
	cases := map[int64]string{
		1000 + 11*3600 + 32*60: "in 11h 32m",
		1000 + 40*60:           "in 40 min",
		900:                    "expired",
	}
	for expiry, want := range cases {
		if got := leaseIn(expiry, now); got != want {
			t.Errorf("leaseIn(%d) = %q, want %q", expiry, got, want)
		}
	}
}

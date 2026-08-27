// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
)

const testLeases = "1787825263 42:e6:ad:ff:b7:af 192.168.77.102 toms-iphone 01:42:e6:ad:ff:b7:af\n" +
	"1787825264 a2:8c:d7:4a:0e:57 192.168.77.120 * *\n" +
	"1787825265 06:11:22:33:44:55 192.168.77.130 old-printer *\n"

// testConnStats: what the helper's connStats verb reports — per-address
// totals, one row per family for the same device.
func testConnStats() map[string]sysstat.DeviceTraffic {
	return map[string]sysstat.DeviceTraffic{
		"192.168.77.102":        {TxBytes: 5000, RxBytes: 90000, Conns: 1},
		"fd42:7ea:aa00:0:1::66": {TxBytes: 80, RxBytes: 200, Conns: 1},
		"10.0.0.9":              {TxBytes: 1 << 40, RxBytes: 1 << 40, Conns: 9}, // nobody's — ignored
	}
}

// testNeighbors: .102 is confirmed (reachable) and carries v6 entries on the
// same MAC, .120 has a lapsed entry (stale), .130 has none at all.
func testNeighbors() ([]sysstat.Neighbor, error) {
	return []sysstat.Neighbor{
		{Addr: "192.168.77.102", MAC: "42:e6:ad:ff:b7:af", State: 0x02},        // reachable
		{Addr: "fd42:7ea:aa00:0:1::66", MAC: "42:e6:ad:ff:b7:af", State: 0x04}, // its v6, stale
		{Addr: "fe80::44:11", MAC: "42:e6:ad:ff:b7:af", State: 0x04},           // its link-local
		{Addr: "192.168.77.120", MAC: "a2:8c:d7:4a:0e:57", State: 0x04},        // stale
		{Addr: "fe80::dead", MAC: "", State: 0x20},                             // failed, unresolved
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
	be := rosterBackend()
	be.connStats = testConnStats()
	s := newServer(t, be)
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	s.bridgePorts = func() (map[string]string, error) {
		return map[string]string{"42:e6:ad:ff:b7:af": "lan0"}, nil
	}
	return s
}

func rosterDevices(t *testing.T, s *Server) []deviceEntry {
	t.Helper()
	return s.deviceList(context.Background(), "test-sid")
}

// TestDeviceList: leases become friendly entries — presence from the
// kernel's confidence across all the MAC's addresses, the zone from the
// firewall config, the port from the bridge FDB, traffic folded across both
// families from conntrack.
func TestDeviceList(t *testing.T) {
	devices := rosterDevices(t, rosterServer(t))
	if len(devices) != 3 {
		t.Fatalf("got %d devices, want 3: %+v", len(devices), devices)
	}
	online := devices[0]
	if online.Name != "toms-iphone" || online.IP != "192.168.77.102" ||
		online.Presence != presenceOnline || online.Icon != "phone" {
		t.Errorf("online device = %+v", online)
	}
	if online.Zone != "lan" || online.Port != "lan0" || online.LeaseExpiry != 1787825263 {
		t.Errorf("online device enrichment = %+v", online)
	}
	if v6 := online.ipv6(); len(v6) != 2 || v6[0] != "fd42:7ea:aa00:0:1::66" || v6[1] != "fe80::44:11" {
		t.Errorf("online device v6 = %v, want global first then link-local", v6)
	}
	if online.Traffic.RxBytes != 90200 || online.Traffic.TxBytes != 5080 || online.Traffic.Conns != 2 {
		t.Errorf("online device traffic = %+v, want both families folded", online.Traffic)
	}
	idle := devices[1]
	if idle.Name != "Device 0e:57" || idle.Presence != presenceIdle || idle.Icon != "device" {
		t.Errorf("idle device = %+v", idle)
	}
	offline := devices[2]
	if offline.Name != "old-printer" || offline.Presence != presenceOffline {
		t.Errorf("offline device = %+v", offline)
	}
}

// TestDeviceListDegrades: no lease file means no roster; every other source
// failing keeps the roster with the facts that remain.
func TestDeviceListDegrades(t *testing.T) {
	s := rosterServer(t)
	s.readLeases = func() ([]byte, error) { return nil, errors.New("no dnsmasq") }
	if devices := rosterDevices(t, s); devices != nil {
		t.Fatalf("no leases: got %+v, want none", devices)
	}

	s = rosterServer(t)
	be := rosterBackend()
	be.connErr = errors.New("no conntrack")
	s.backend = be
	s.neighbors = func() ([]sysstat.Neighbor, error) { return nil, errors.New("no netlink") }
	s.bridgePorts = func() (map[string]string, error) { return nil, errors.New("no fdb") }
	for _, d := range rosterDevices(t, s) {
		if d.Presence != presenceOffline || d.Port != "" || d.Traffic.Conns != 0 {
			t.Fatalf("kernel down: %+v, want bare lease facts", d)
		}
	}
}

// TestDeviceIcon: the silhouette is a hostname hint, generic when nothing
// matches.
func TestDeviceIcon(t *testing.T) {
	cases := map[string]string{
		"Galaxy-S24": "phone", "living-room-tv": "tv", "MacBook-Pro": "laptop",
		"ap-attic": "router", "mystery-box": "device",
	}
	for host, want := range cases {
		if got := deviceIcon(host); got != want {
			t.Errorf("deviceIcon(%q) = %q, want %q", host, got, want)
		}
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

// TestIndexRendersRoster: the section sits bare on the canvas — trigger rows
// with zone chips, presence badges, honest chevrons — and each device's wide
// drawer carries the kernel's full story.
func TestIndexRendersRoster(t *testing.T) {
	body := get(t, rosterServer(t), "/").Body.String()
	for _, want := range []string{
		"Connected devices", "toms-iphone", "192.168.77.102", "Device 0e:57",
		"Online", "Idle", "Offline",
		">lan<",                 // the zone chip
		"max-w-2xl",             // the wide drawer
		"fd42:7ea:aa00:0:1::66", // v6 in the drawer's address table
		"42:e6:ad:ff:b7:af",     // MAC in identity
		"lan0",                  // bridge port
		"Active connections",    // conntrack block
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
}

// TestIndexWithoutLeasesSkipsRoster: a router not serving DHCP carries no
// empty section.
func TestIndexWithoutLeasesSkipsRoster(t *testing.T) {
	if strings.Contains(get(t, newServer(t, fakeBackend{}), "/").Body.String(), "Connected devices") {
		t.Error("GET /: roster rendered with no leases")
	}
}

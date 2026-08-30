// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
)

// NUD states as raw kernel bits (reachable / stale), cast through the exported
// type — enough to exercise the plain-language mapping without a live netlink.
const (
	nudReachable = sysstat.NeighState(0x02)
	nudStale     = sysstat.NeighState(0x04)
)

func TestDeviceAddresses(t *testing.T) {
	l := lease{ip: "192.168.1.10", mac: "aa:bb:cc:dd:ee:ff", host: "laptop"}
	entries := []sysstat.Neighbor{
		{Addr: "192.168.1.10", State: nudReachable}, // same as the lease — deduped
		{Addr: "2001:db8::5", State: nudReachable},  // global v6 — the primary
		{Addr: "fe80::1", State: nudStale},          // link-local — never primary
	}

	v4, v6, all := deviceAddresses(l, true, entries)
	if v4 != "192.168.1.10" {
		t.Errorf("primary v4 = %q", v4)
	}
	if v6 != "2001:db8::5" {
		t.Errorf("primary v6 should skip link-local, got %q", v6)
	}
	if len(all) != 3 {
		t.Fatalf("addresses should dedup the lease IP: got %d (%+v)", len(all), all)
	}
	if all[0].Addr != "192.168.1.10" || all[0].Family != "IPv4" || all[0].State != "reachable" {
		t.Errorf("first address wrong: %+v", all[0])
	}
	if all[2].Family != "IPv6" || all[2].State != "stale" {
		t.Errorf("link-local address wrong: %+v", all[2])
	}
}

func TestDeviceAddressesV6Only(t *testing.T) {
	// A device with no lease and only a ULA v6 — still a device, v6 is primary.
	v4, v6, all := deviceAddresses(lease{}, false, []sysstat.Neighbor{
		{Addr: "fd00::9", State: nudReachable},
	})
	if v4 != "" {
		t.Errorf("v4 should be empty, got %q", v4)
	}
	if v6 != "fd00::9" {
		t.Errorf("primary v6 = %q", v6)
	}
	if len(all) != 1 {
		t.Errorf("addresses = %+v", all)
	}
}

func TestNeighborInterface(t *testing.T) {
	entries := []sysstat.Neighbor{
		{Addr: "2001:db8::5", Interface: "br-lan.10"},
		{Addr: "192.168.10.25", Interface: "br-lan.10"},
	}
	if got := neighborInterface(entries, "192.168.10.25"); got != "br-lan.10" {
		t.Errorf("interface = %q", got)
	}
	if got := neighborInterface(entries, ""); got != "br-lan.10" {
		t.Errorf("fallback interface = %q", got)
	}
}

func TestZonesByDeviceCoversAddresslessNeighbors(t *testing.T) {
	cfg := rosterBackend().uci
	if got := zonesByDevice(cfg["network"], cfg["firewall"])["br-lan"]; got != "lan" {
		t.Errorf("br-lan zone = %q", got)
	}
}

func TestDeviceFactHelpers(t *testing.T) {
	if got := presenceWord(presenceOnline); got != "online" {
		t.Errorf("presenceWord(online) = %q", got)
	}
	if got := leaseFact(lease{}, false, time.Time{}); got != "No DHCP lease" {
		t.Errorf("no-lease fact = %q", got)
	}
}

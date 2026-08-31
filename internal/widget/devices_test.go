// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// roster is the listing in miniature: a leased device that could be reserved, a
// device whose address is already reserved, and one the box has only seen.
func roster() []Device {
	return []Device{
		{
			Name: "Gaming PC", Icon: "gamepad", MAC: "a4:83:e7:2b:19:0c",
			DUID: "00:03:00:01:a4:83:e7:2b:19:0c",
			V4:   "192.168.1.104", V6: "2001:db8::4f",
			Interface: "br-lan", Zone: "lan", Presence: "online",
			Addresses: []DeviceAddr{
				{Addr: "192.168.1.104", Family: "IPv4", State: "reachable"},
				{Addr: "2001:db8::4f", Family: "IPv6", State: "reachable"},
			},
			Connection: "lan1", Lease: "in 11h 12m",
			Leased: true, ReserveHref: "/plugins/dnsdhcp/?reserve=a4%3A83%3Ae7%3A2b%3A19%3A0c",
		},
		{
			Name: "nas", MAC: "de:ad:be:ef:00:11", V4: "192.168.1.10",
			Interface: "br-lan", Zone: "lan", Presence: "idle",
			Addresses: []DeviceAddr{{Addr: "192.168.1.10", Family: "IPv4", State: "stale"}},
			Lease:     "in 9h 02m", Leased: true, Reserved: true,
			ReserveHref: "/plugins/dnsdhcp/?reserve=de%3Aad%3Abe%3Aef%3A00%3A11",
		},
		{
			Name: "Unknown device", MAC: "9e:2f:11:c4:08:5b", V4: "192.168.20.44",
			Interface: "br-lan.20", Zone: "guest", Presence: "offline",
			Addresses: []DeviceAddr{{Addr: "192.168.20.44", Family: "IPv4", State: "stale"}},
			Lease:     "No DHCP lease",
		},
	}
}

// TestRenderDevicesTable: the roster is one flat listing — name, interface with
// its zone chip, MAC, address, presence — and every row opens the full story.
func TestRenderDevicesTable(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(roster()))
	for _, want := range []string{
		"Connected devices", "3 devices", // the header band and its count
		"a4:83:e7:2b:19:0c", "192.168.1.104", "2001:db8::4f",
		"br-lan", "br-lan.20", // the kernel segment
		"M20 13c0 5-3.5 7.5", // zone icon → the shared firewall-zone chip
		"Online", "Idle", "Offline",
		"Details", "max-w-2xl", // the drawer opener and its wide panel
		"in 11h 12m", "No DHCP lease", // drawer lease facts
		"DUID", "00:03:00:01:a4:83:e7:2b:19:0c",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("devices table missing %q", want)
		}
	}
}

// TestRenderDevicesDrawerReserveDoor: only a device holding an address it does
// not already own is offered the reservation — a reserved one states the fact
// instead, and a device with no lease has nothing to keep.
func TestRenderDevicesDrawerReserveDoor(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(roster()))
	if strings.Count(got, "Reserve its address") != 1 {
		t.Errorf("exactly the leased, unreserved device should be offered a reservation:\n%s", got)
	}
	if !strings.Contains(got, `href="/plugins/dnsdhcp/?reserve=a4%3A83%3Ae7%3A2b%3A19%3A0c"`) {
		t.Errorf("the reserve door should lead to the DHCP page with the device's MAC:\n%s", got)
	}
	if !strings.Contains(got, "Address reserved for this device") {
		t.Errorf("a reserved device should state its reservation")
	}
	if strings.Count(got, "Address reserved for this device") != 1 {
		t.Errorf("only the reserved device should state a reservation")
	}
}

// TestRenderDevicesTableEmpty: a roster with nobody on it says so in its own
// words rather than drawing column headings over nothing.
func TestRenderDevicesTableEmpty(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(nil))
	if !strings.Contains(got, "Nothing has joined this network yet.") {
		t.Errorf("empty roster missing its own empty text:\n%s", got)
	}
	if strings.Contains(got, "<thead") {
		t.Errorf("empty roster should draw no column headings:\n%s", got)
	}
}

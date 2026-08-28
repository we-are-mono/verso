// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderOverview: the overview draws every section — verdict, tiles, IPv4/IPv6
// facts (with copy), the injected traffic chart, System, and the injected flat-Table
// listings (Interfaces, DHCP leases) — with the headline in the serif display face.
// The live System facts are filled from the fields; an empty Overview shows them as
// "unavailable".
func TestRenderOverview(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Overview{
		Model:    "Mono Gateway Development Kit",
		Firmware: "OpenWrt 25.12.4", Kernel: "Linux 6.12.101", Uptime: "6d 4h 0m",
		Temperature: "52 °C · Normal", TempDot: "emerald",
		Fan: "3630 rpm", Power: "12.4 W", SensorSummary: "8 power · 5 thermal",
		V4Proto: "DHCP", V4: []OverviewFact{{Label: "Address", Value: "172.30.1.171/24", Copy: true}},
		V6Proto: "DHCPv6 client", V6: []OverviewFact{{Label: "Prefix", Value: "fd42:7ea:aa00::/56", Copy: true}},
		SysMetrics: []OverviewMeter{
			{Name: "sys-load", Label: "LOAD", Icon: "activity", Role: "sky", Value: "1.16", Fill: 29},
			{Name: "sys-cpu", Label: "CPU", Icon: "cpu", Role: "violet", Value: "27", Unit: "%", Fill: 27},
			{Name: "sys-memory", Label: "MEMORY", Icon: "memory-stick", Role: "emerald", Value: "60", Unit: "%", Fill: 60},
			{Name: "sys-storage", Label: "STORAGE", Icon: "hard-drive", Role: "amber", Value: "78", Unit: "%", Fill: 78},
		},
		Ports: []OverviewPort{
			{Port: "eth0", Up: false, Speed: "", Traffic: ""},
			{Port: "eth4", Up: true, Wan: true, VLANs: "10, 20", Speed: "10 Gbps", Traffic: "18.4 / 2.1 GB"},
		},
		Interfaces: []OverviewInterface{
			{
				Name: "lan", Subnet: "192.168.1.0/24", Zone: "lan", Ports: "lan1, lan2", Devices: "8",
				Proto: "static", Device: "br-lan",
				PortDetail: []OverviewInterfacePort{{Port: "lan1", Mode: "untagged"}, {Port: "lan2", Mode: "untagged"}},
			},
			{
				Name: "guest", VLAN: "20", Subnet: "192.168.20.0/24", Zone: "guest", Ports: "lan3, lan4",
				Proto: "static", Device: "br-lan.20",
				PortDetail: []OverviewInterfacePort{{Port: "lan3", Mode: "tagged"}, {Port: "lan4", Mode: "untagged"}},
			},
		},
		Devices: []OverviewDevice{
			{
				Name: "Gaming PC", MAC: "a4:83:e7:2b:19:0c",
				DUID: "00:03:00:01:a4:83:e7:2b:19:0c",
				V4:   "192.168.1.104", V6: "2001:db8::4f",
				Interface: "lan", Zone: "lan", Presence: "online",
				Addresses: []OverviewAddr{
					{Addr: "192.168.1.104", Family: "IPv4", State: "reachable"},
					{Addr: "2001:db8::4f", Family: "IPv6", State: "reachable"},
				},
				Connection: "lan1", Lease: "in 11h 12m",
				Traffic: "12.4 GB down · 3.1 GB up", Conns: "42",
			},
			{
				Name: "Unknown device", MAC: "9e:2f:11:c4:08:5b",
				V4: "192.168.20.44", Interface: "guest", Zone: "guest", Presence: "offline",
				Addresses: []OverviewAddr{{Addr: "192.168.20.44", Family: "IPv4", State: "stale"}},
				Lease:     "No DHCP lease",
			},
		},
	})
	for _, want := range []string{
		"ALL GOOD", "healthy", "font-serif", // verdict in Fraunces
		"Basic", "Advanced", // the view toggle
		"INTERNET", "WI-FI", "SECURITY", "SOFTWARE", // status tiles
		"IPV4", "172.30.1.171/24", "IPV6", "fd42:7ea:aa00::/56",
		"x-data=\"copy\"",                             // copy control on the IP values
		"Internet traffic", "live · WAN", "Mbps down", // chart header + legend
		"verso-chart", "verso-chart-area", // the injected chart + its gradient fill
		"System", "LOAD", "CPU", "MEMORY", "STORAGE",
		"data-verso-meter=\"sys-cpu\"", // named gauge, so the stream can update it
		"Mono Gateway Development Kit",
		"OpenWrt 25.12.4", "Linux 6.12.101", "6d 4h 0m", // live System facts
		// Resolved hardware sensors (profile-keyed); the temp dot reads emerald.
		"Temperature", "52 °C · Normal", "bg-emerald-500",
		"Fan", "3630 rpm", "Power draw", "12.4 W", "8 power · 5 thermal",
		// Ports table (flat Table): the WAN tag rides the eth4 link cell, eth0's
		// down row greys, the VLANs column lists carried ids, and the header
		// counts the ports.
		"Ports", "2 ports", "eth0", "No link", "eth4", ">WAN<", "18.4 / 2.1 GB",
		"VLANs", "10, 20", // the port's carried VLANs
		// Interfaces table: logical networks with VLAN id, subnet, zone chip, and a
		// Details drawer carrying the per-port tagged/untagged membership.
		"Interfaces", "2 interfaces", "guest", "192.168.20.0/24", "192.168.1.0/24",
		"tagged", "untagged",
		// Entity reference chips carry their Lucide type icon so interface / zone /
		// port never blur: network glyph on the interface chip, ethernet-port glyph
		// on the port chips.
		"M12 12V8", // network icon → the device's Interface chip
		"M10 8v1",  // ethernet-port icon → the interface's port chips
		// Connected devices: MAC + stacked v4/v6 addresses, presence, and the
		// per-row Details drawer carrying the full story.
		"Connected devices", "2 devices",
		"a4:83:e7:2b:19:0c", "192.168.1.104", "2001:db8::4f", // MAC + stacked addresses
		"Interface",         // the segment column (ties to the Interfaces table)
		"Online", "Offline", // presence words
		"Details", "max-w-2xl",             // the drawer opener + its wide panel
		"in 11h 12m", "No DHCP lease",              // drawer lease facts
		"12.4 GB down · 3.1 GB up", "lan1",         // drawer traffic + connection
		"DUID", "00:03:00:01:a4:83:e7:2b:19:0c", // the DHCPv6 identity in the drawer
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	// The software tile reads amber; the warning stops nowhere else recolours the number.
	if !strings.Contains(got, "text-amber-700") {
		t.Errorf("the software (update) tile should read amber")
	}
	// A missing live fact degrades to "unavailable" rather than a stale placeholder,
	// and an unreadable sensor drops its row entirely (no fabricated "Power draw").
	bare := render(t, r, &Overview{})
	if !strings.Contains(bare, "unavailable") {
		t.Errorf("an empty overview should mark its live facts unavailable")
	}
	if strings.Contains(bare, "Power draw") {
		t.Errorf("an unreadable power sensor should hide its row, not show a zero")
	}
}

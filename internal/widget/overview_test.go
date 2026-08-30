// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderOverview: the overview draws every section — verdict, tiles, IPv4/IPv6
// facts (with copy), the injected traffic chart, System, and the injected flat-Table
// listings (Interfaces, Connected devices) — with the headline in the serif display face.
// The live System facts are filled from the fields; an empty Overview shows them as
// "unavailable".
func TestRenderOverview(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Overview{
		WiFiPresent: true,
		Model:       "Mono Gateway Development Kit",
		Firmware:    "OpenWrt 25.12.4", Kernel: "Linux 6.12.101", Uptime: "6d 4h 0m",
		WANKnown: true, WANUp: true, WANUptime: "2h 14m",
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
		Interfaces: []OverviewInterface{
			{
				Name: "eth0", Kind: "port", State: "down", Physical: true,
				RxRate: "0 bps", TxRate: "0 bps", RxTotal: "0 B", TxTotal: "0 B",
				RxPackets: "0 pkt/s", TxPackets: "0 pkt/s",
			},
			{
				Name: "eth4", Kind: "port", State: "up", Physical: true, WAN: true, Zone: "wan",
				Relations: []OverviewInterfaceRelation{{Name: "eth4.3900"}},
				RxRate:    "18.4 Mbps", TxRate: "2.1 Mbps", RxTotal: "18.4 GiB", TxTotal: "2.1 GiB",
				RxPackets: "1200 pkt/s", TxPackets: "180 pkt/s",
			},
			{
				Name: "br-lan.20", Kind: "vlan", State: "up", Networks: []string{"guest"},
				VLAN: "20", Subnet: "192.168.20.0/24", Zone: "guest", Proto: "static",
				Relations: []OverviewInterfaceRelation{{Name: "br-lan"}, {Name: "eth3", Physical: true}},
				RxRate:    "115 Kbps", TxRate: "0 bps", RxTotal: "6.8 GiB", TxTotal: "4.6 GiB",
				RxPackets: "13 pkt/s", TxPackets: "0 pkt/s",
			},
			{
				Name: "uap0", Kind: "wifi", State: "up", Networks: []string{"lan"}, Zone: "lan",
				RxRate: "8.2 Mbps", TxRate: "1.4 Mbps", RxTotal: "2.1 GiB", TxTotal: "640 MiB",
				RxPackets: "820 pkt/s", TxPackets: "210 pkt/s",
			},
		},
		Devices: []OverviewDevice{
			{
				Name: "Gaming PC", MAC: "a4:83:e7:2b:19:0c",
				DUID: "00:03:00:01:a4:83:e7:2b:19:0c",
				V4:   "192.168.1.104", V6: "2001:db8::4f",
				Interface: "br-lan", Zone: "lan", Presence: "online",
				Addresses: []OverviewAddr{
					{Addr: "192.168.1.104", Family: "IPv4", State: "reachable"},
					{Addr: "2001:db8::4f", Family: "IPv6", State: "reachable"},
				},
				Connection: "lan1", Lease: "in 11h 12m",
				Traffic: "12.4 GB down · 3.1 GB up", Conns: "42",
			},
			{
				Name: "Unknown device", MAC: "9e:2f:11:c4:08:5b",
				V4: "192.168.20.44", Interface: "br-lan.20", Zone: "guest", Presence: "offline",
				Addresses: []OverviewAddr{{Addr: "192.168.20.44", Family: "IPv4", State: "stale"}},
				Lease:     "No DHCP lease",
			},
		},
	})
	for _, want := range []string{
		"ALL GOOD", "healthy", "font-serif", // verdict in Fraunces
		"Basic", "Advanced", // the view toggle
		"INTERNET", "for 2h 14m", "data-verso-tile-caption=\"internet-uptime\"",
		"WI-FI", "SECURITY", "SOFTWARE", // status tiles
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
		// One telemetry-backed interface table contains physical and logical
		// devices. A physical interface carries the requested RJ45 "port" chip.
		"Interfaces", "4 interfaces", "eth0", "eth4", "br-lan.20", "uap0",
		"Ethernet", "VLAN", "Topology", ">RX<", ">TX<",
		">port<", ">WAN<", "18.4 Mbps", "2.1 Mbps", "192.168.20.0/24",
		// Entity reference chips carry their Lucide type icon so interface / zone /
		// port never blur: network glyph on the interface chip, ethernet-port glyph
		// on the port chips.
		"M12 12V8",           // network icon → an interface topology chip
		"M10 8v1",            // ethernet-port icon → the interface's port chips
		"M20 13c0 5-3.5 7.5", // zone icon → the shared firewall-zone chip
		// Connected devices: MAC + stacked v4/v6 addresses, presence, and the
		// per-row Details drawer carrying the full story.
		"Connected devices", "2 devices",
		"a4:83:e7:2b:19:0c", "192.168.1.104", "2001:db8::4f", // MAC + stacked addresses
		"Interface", "br-lan", "br-lan.20", // kernel segment + zone chip
		"Online", "Offline", // presence words
		"Details", "max-w-2xl", // the drawer opener + its wide panel
		"in 11h 12m", "No DHCP lease", // drawer lease facts
		"12.4 GB down · 3.1 GB up", "lan1", // drawer traffic + connection
		"DUID", "00:03:00:01:a4:83:e7:2b:19:0c", // the DHCPv6 identity in the drawer
		"dark:border-gray-700 dark:bg-gray-800", // dark segmented-control track
		"dark:bg-gray-700 dark:text-gray-100",   // active Advanced segment
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	for _, want := range []string{
		`font-mono text-base font-semibold text-slate-900">172.30.1.171/24`,
		`font-mono text-base font-semibold tabular-nums whitespace-nowrap text-slate-900 group-last:border-b-0">a4:83:e7:2b:19:0c`,
		`font-mono text-base font-semibold tabular-nums group-last:border-b-0`,
		`class="text-base font-medium text-slate-900"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview monospace value missing fixed Tailwind typography %q:\n%s", want, got)
		}
	}
	// The software tile reads amber; the warning stops nowhere else recolours the number.
	if !strings.Contains(got, "text-amber-700") {
		t.Errorf("the software (update) tile should read amber")
	}
	if !strings.Contains(got, "</svg></span>port</span>") {
		t.Errorf("a physical interface should carry an RJ45 port chip")
	}
	if strings.Contains(got, "Network / role") {
		t.Errorf("the sparse network/role column should not be rendered")
	}
	if !strings.Contains(got, "[&_td]:align-top") {
		t.Errorf("interface table cells should be aligned to the top")
	}
	if strings.Count(got, "w-32 min-w-32 max-w-32") < 4 {
		t.Errorf("the live RX and TX columns should both have a stable fixed width")
	}
	if !strings.Contains(got, "grid grid-cols-4") {
		t.Errorf("Wi-Fi hardware should render the four-column status strip")
	}
	if !strings.Contains(got, "</svg></span>Wi-Fi</span>") {
		t.Errorf("a wireless interface should carry a Wi-Fi chip")
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
	if strings.Contains(bare, "WI-FI") || !strings.Contains(bare, "grid grid-cols-3") {
		t.Errorf("an overview without Wi-Fi hardware should use three full-width status columns")
	}
}

// TestRenderOverviewTrafficAutoScale: WAN traffic uses the tallest value across
// both directions, with headroom, instead of clipping to the former fixed range.
func TestRenderOverviewTrafficAutoScale(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Overview{
		DownSeries: []float64{80, 400, 120},
		UpSeries:   []float64{20, 40, 30},
	})
	for _, want := range []string{">460 Mbps<", ">345<", ">230<", ">115<"} {
		if !strings.Contains(got, want) {
			t.Errorf("auto-scaled overview chart missing %q:\n%s", want, got)
		}
	}
}

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
		// Interfaces table (flat Table): the WAN tag rides the eth4 link cell.
		"Interfaces", "eth0", "eth4", ">WAN<", "18.4 / 2.1 GB",
		// DHCP leases table: MAC + IP mono, copyable.
		"DHCP leases", "5 active", "a4:83:e7:2b:19:0c", "192.168.20.44",
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

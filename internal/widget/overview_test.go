// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRenderOverview(t *testing.T) {
	got := render(t, newRenderer(t), &Overview{
		WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min",
		InterfacesKnown: true, DevicesKnown: true, DevicesOnline: 9,
		SecurityHref: "/plugins/firewall/", Model: "Mono Gateway Development Kit",
		TunnelsHref: "/plugins/wireguard/",
		Firmware:    "OpenWrt 25.12.4", Kernel: "Linux 6.12.101", Uptime: "6 d 04:00",
		Clock: "14:52:07", Zone: "CEST",
		Temperature: "52 °C", TempDot: "success", Fan: "3630 rpm", Power: "12.4 W",
		V4Proto: "DHCP", V4: []OverviewFact{{Label: "Address", Value: "172.30.1.171/24", Copy: true}},
		V6Proto: "DHCPv6 client", V6: []OverviewFact{{Label: "Prefix", Value: "fd42:7ea:aa00::/56", Copy: true}},
		SysMetrics: []OverviewMeter{{Name: "sys-cpu", Label: "CPU", Icon: "cpu", Value: "27", Unit: "%", Fill: 27, Band: "success"}},
		Interfaces: []OverviewInterface{
			{Name: "eth0", Kind: "port", State: "up", Physical: true, Networks: []string{"wan"}, RxRate: "18.4 Mbps", TxRate: "2.1 Mbps"},
			{Name: "wg-home", Kind: "tunnel", State: "up", Networks: []string{"home"}, Proto: "wireguard", Subnet: "10.200.0.1/24"},
		},
	})
	for _, want := range []string{
		`data-overview`, "Internet is working", `class="text-green">online`, "for 2 h 14 min", "pppoe-wan",
		"Firewall", `href="/plugins/firewall/"`, "Tunnels", "All ports linked", "2 interfaces", "9 devices connected",
		"IPv4", "dhcp", "172.30.1.171/24", "IPv6", "dhcpv6", "fd42:7ea:aa00::/56", `x-data="copy"`,
		"Internet traffic", "Mbit/s down", "Mbit/s up", "verso-chart-area", "verso-chart--emerald", `data-chart-padding="0"`,
		`href="/plugins/wireguard/"`,
		"System", "CPU", `data-verso-meter="sys-cpu"`, "Mono Gateway Development Kit", "OpenWrt 25.12.4", "Linux 6.12.101",
		"14:52:07", "CEST", "6 d 04:00", "CPU Temperature", "Fan speed", "Power draw", "52 °C", "3630 rpm", "12.4 W",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(got, "(two)") || strings.Contains(got, "(few)") {
		t.Error("plural context leaked into English")
	}
	if strings.Contains(got, `id="overview-interfaces"`) || strings.Contains(got, "<table") {
		t.Error("the homepage must not render an interface inventory")
	}
	if strings.Contains(got, "overview-tunnels") || strings.Contains(got, "overview-system-title") {
		t.Error("the homepage must not render the tunnel inventory or System header")
	}
	if strings.Contains(got, "Protected") {
		t.Error("an answering plugin is not proof the firewall protects the network")
	}
}

func TestOverviewDegradesWithoutInventingState(t *testing.T) {
	r := newRenderer(t)
	bare := render(t, r, &Overview{})
	for _, want := range []string{"Status unavailable", "Unavailable", "N/A", "CPU Temperature", "Fan speed", "Power draw", "System readings unavailable"} {
		if !strings.Contains(bare, want) {
			t.Errorf("missing fallback %q", want)
		}
	}
	for _, falseClaim := range []string{"healthy", "All ports linked", "All interfaces up", "Both bands active"} {
		if strings.Contains(bare, falseClaim) {
			t.Errorf("unknown router claims %q", falseClaim)
		}
	}
	down := render(t, r, &Overview{WANKnown: true, WANUp: false, InterfacesKnown: true})
	if !strings.Contains(down, "Your network is offline") || !strings.Contains(down, `data-tone="danger"`) {
		t.Error("offline WAN must change the verdict and tile")
	}
	unknown := render(t, r, &Overview{InterfacesKnown: true, Interfaces: []OverviewInterface{{Name: "wg0", Kind: "tunnel", State: "unknown", Networks: []string{"vpn"}, Proto: "wireguard"}}})
	if !strings.Contains(unknown, "State unknown") || strings.Contains(unknown, "All interfaces up") {
		t.Error("a tunnel's unknown kernel state is not a connected peer")
	}
}

func TestOverviewConnectionAbsenceKeepsIPv6Column(t *testing.T) {
	got := render(t, newRenderer(t), &Overview{V4: []OverviewFact{{Label: "Address", Value: "192.0.2.1/24", Copy: true}}, V6: []OverviewFact{{Label: "Status", Value: "Not configured"}}})
	if !strings.Contains(got, `aria-label="IPv6"`) || !strings.Contains(got, "No IPv6 connection · no prefix was delegated") {
		t.Error("missing IPv6 needs its own retained column")
	}
}

func TestRenderOverviewTrafficAutoScale(t *testing.T) {
	got := render(t, newRenderer(t), &Overview{DownSeries: []float64{80, 400, 120}, UpSeries: []float64{20, 40, 30}})
	for _, want := range []string{">460 Mbit/s<", ">345<", ">230<", ">115<", `viewBox="0 0 620 240"`, `data-chart-padding="0"`} {
		if !strings.Contains(got, want) {
			t.Errorf("existing shared-scale renderer missing %q", want)
		}
	}
}

func TestOverviewStatusFollowsReadings(t *testing.T) {
	tr := func(s string) string { return s }
	o := &Overview{WANKnown: true, WANUp: true, InterfacesKnown: true, Interfaces: []OverviewInterface{{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}}, {Name: "wg0", Kind: "tunnel", State: "up", Networks: []string{"vpn"}}}}
	status := o.LiveStatus(tr)
	if status.Tone != "success" || status.Accent != "online" {
		t.Fatalf("up = %+v", status)
	}
	o.Interfaces[1].State = "down"
	status = o.LiveStatus(tr)
	if status.Tone != "warning" || status.Kicker != "1 needs attention" || status.Tiles[2].Tone != "danger" {
		t.Fatalf("tunnel down = %+v", status)
	}
	o.Interfaces[1].State = "up"
	o.SysMetrics = []OverviewMeter{{Band: "danger"}}
	if status = o.LiveStatus(tr); status.Tone != "warning" {
		t.Fatalf("resource at critical = %+v", status)
	}
	o.WANUp = false
	if status = o.LiveStatus(tr); status.Tone != "danger" || status.Lead != "Your network is offline" {
		t.Fatalf("WAN down = %+v", status)
	}
	o.WANKnown = false
	if status = o.LiveStatus(tr); status.Tone != "neutral" || status.Kicker != "Status unavailable" {
		t.Fatalf("read failed = %+v", status)
	}
	o.Interfaces[0].State = "unknown"
	if tile := o.interfacesTile(tr); tile.Status == "All ports linked" {
		t.Fatal("unknown link state claimed connected")
	}
}

// The kernel brings sit0 and ip6tnl0 up on its own, and a spare port has no
// cable: down, but no network asked for them, so nothing needs attention.
func TestOverviewStatusIgnoresUnusedInterfaces(t *testing.T) {
	tr := func(s string) string { return s }
	o := &Overview{WANKnown: true, WANUp: true, InterfacesKnown: true, Interfaces: []OverviewInterface{
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}},
		{Name: "lan1", Physical: true, State: "down", Relations: []OverviewInterfaceRelation{{Name: "br-lan"}}},
		{Name: "lan4", Physical: true, State: "down"},
		{Name: "sit0", Kind: "tunnel", State: "down"},
		{Name: "ip6tnl0", Kind: "tunnel", State: "down"},
	}}
	status := o.LiveStatus(tr)
	if status.Kicker != "1 needs attention" {
		t.Fatalf("kicker = %q, want only the bridged lan1 counted", status.Kicker)
	}
	if tile := o.tunnelTile(tr); tile.Status != "None observed" {
		t.Fatalf("tunnel tile = %+v, want unused tunnels left out", tile)
	}
	if tile := o.interfacesTile(tr); tile.Identity != "lan1" {
		t.Fatalf("interfaces tile = %+v, want lan1 named, not the spare lan4", tile)
	}
}

func TestOverviewCountGrammarAndFallback(t *testing.T) {
	catalog := map[string]string{"%d interface": "%d vmesnik", "%d interfaces (two)": "%d vmesnika", "%d interfaces (few)": "%d vmesniki", "%d interfaces": "%d vmesnikov"}
	tr := func(key string) string {
		if v, ok := catalog[key]; ok {
			return v
		}
		return key
	}
	for _, tc := range []struct {
		n      int
		en, sl string
	}{{0, "0 interfaces", "0 vmesnikov"}, {1, "1 interface", "1 vmesnik"}, {2, "2 interfaces", "2 vmesnika"}, {3, "3 interfaces", "3 vmesniki"}, {4, "4 interfaces", "4 vmesniki"}, {5, "5 interfaces", "5 vmesnikov"}, {101, "101 interfaces", "101 vmesnik"}, {102, "102 interfaces", "102 vmesnika"}} {
		if got := interfaceCount(func(s string) string { return s }, tc.n); got != tc.en {
			t.Errorf("en(%d)=%q", tc.n, got)
		}
		if got := interfaceCount(tr, tc.n); got != tc.sl {
			t.Errorf("sl(%d)=%q", tc.n, got)
		}
	}
}

func TestOverviewFirewallPageAndLiveStates(t *testing.T) {
	tr := func(s string) string { return s }
	for _, tc := range []struct {
		state, status, tone, caption, verdict string
		rules                                 int
	}{
		{"", "Status unavailable", "neutral", "Review firewall settings", "success", 0},
		{"active", "Active", "success", "1 configured rule", "success", 1},
		{"active", "Active", "success", "53 configured rules", "success", 53},
		{"inactive", "Inactive", "danger", "Firewall is not running", "warning", 0},
		{"partial", "Incomplete", "warning", "Some filter chains are missing", "warning", 12},
	} {
		t.Run(tc.status+tc.caption, func(t *testing.T) {
			o := &Overview{WANKnown: true, WANUp: true, SecurityHref: "/plugins/firewall/", FirewallState: tc.state, FirewallRules: tc.rules, FirewallRulesKnown: true}
			tile := o.firewallTile(tr)
			if tile.Status != tc.status || tile.Variant != tc.tone || tile.Caption != tc.caption {
				t.Fatalf("tile = %+v", tile)
			}
			page := render(t, newRenderer(t), o)
			if !strings.Contains(page, tc.status) || !strings.Contains(page, tc.caption) {
				t.Fatalf("page missing firewall reading %s / %s", tc.status, tc.caption)
			}
			live := o.LiveStatus(tr)
			if len(live.Tiles) != 4 || live.Tiles[1].ID != "firewall" || live.Tiles[1].Status != tile.Status || live.Tiles[1].Tone != tile.Variant || live.Tiles[1].Caption != tile.Caption {
				t.Fatalf("live reading differs from first paint: %+v", live)
			}
			if live.Tone != tc.verdict {
				t.Fatalf("verdict = %s, want %s", live.Tone, tc.verdict)
			}
		})
	}
}

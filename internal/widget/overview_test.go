// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"regexp"
	"strings"
	"testing"
)

func fullOverview() *Overview {
	return &Overview{
		WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min",
		InterfacesKnown: true, DevicesKnown: true, DevicesOnline: 9,
		SecurityHref: "/plugins/firewall/", Model: "Mono Gateway Development Kit",
		TunnelsHref: "/plugins/wireguard/", InterfacesHref: "/plugins/interfaces/",
		Firmware: "OpenWrt 25.12.4", Kernel: "Linux 6.12.101", Uptime: "6 d 04:00",
		Clock: "14:52:07", Zone: "CEST",
		Temperature: "52 °C", TempDot: "success", Fan: "3630 rpm", Power: "12.4 W",
		FirewallState: "active", FirewallRules: 18, FirewallRulesKnown: true,
		V4Proto: "DHCP", V4: []OverviewFact{{Label: "Address", Value: "172.30.1.171/24", Copy: true}},
		V6Proto: "DHCPv6 client", V6: []OverviewFact{{Label: "Prefix", Value: "fd42:7ea:aa00::/56", Copy: true}},
		SysMetrics: []OverviewMeter{{Name: "sys-cpu", Label: "CPU", Icon: "cpu", Value: "27", Unit: "%", Fill: 27, Band: "success"}},
		Interfaces: []OverviewInterface{
			{Name: "eth0", Kind: "port", State: "up", Physical: true, Networks: []string{"wan"}, RxRate: "18.4 Mbps", TxRate: "2.1 Mbps"},
			{Name: "eth4", Kind: "port", State: "down", Physical: true},
			{Name: "wg-home", Kind: "tunnel", State: "up", Networks: []string{"home"}, Proto: "wireguard", Subnet: "10.200.0.1/24"},
		},
	}
}

// cardOrder is the order the page draws its cards in, column by column.
func cardOrder(page string) []string {
	var ids []string
	for _, m := range regexp.MustCompile(`data-overview-card="([a-z]+)"`).FindAllStringSubmatch(page, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func TestRenderOverviewCards(t *testing.T) {
	got := render(t, newRenderer(t), fullOverview())
	for _, want := range []string{
		`data-overview`, `data-overview-tone="success"`, "Your network is", `data-overview-accent class="text-green-deep">online`, "14:52:07", "CEST",
		"Arrange", "Done", "Reset layout",
		// Internet
		"Connected", "for 2 h 14 min · pppoe-wan", "IPv4", "dhcp", "172.30.1.171/24", "IPv6", "dhcpv6", "fd42:7ea:aa00::/56", `x-data="copy"`,
		// Firewall
		"Active", "18 configured rules", `href="/plugins/firewall/"`, "Open Firewall",
		// Ports and Tunnels
		"All ports linked", `data-verso-row="interface:eth0"`, "18.4 Mbps", "Not in use", "wg-home", "wireguard", `href="/plugins/wireguard/"`,
		// Traffic, Resources, Hardware, Firmware
		"Mbit/s down", "verso-chart-area", `data-verso-meter="sys-cpu"`, "Mono Gateway Development Kit", "3630 rpm", "12.4 W",
		"OpenWrt 25.12.4", "Linux 6.12.101", `data-verso-prop="uptime"`, "6 d 04:00",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if want := []string{"internet", "firewall", "ports", "tunnels", "traffic", "resources", "hardware", "firmware"}; strings.Join(cardOrder(got), " ") != strings.Join(want, " ") {
		t.Errorf("default order = %v, want %v", cardOrder(got), want)
	}
	for _, gone := range []string{"Internet is working", "Live overview", "data-overview-tile", "Uptime</span>", "(two)", "(few)"} {
		if strings.Contains(got, gone) {
			t.Errorf("overview still draws %q", gone)
		}
	}
	firmware := got[strings.Index(got, `data-overview-card="firmware"`):]
	firmware = firmware[:strings.Index(firmware, "</article>")]
	if strings.Count(got, "6 d 04:00") != strings.Count(firmware, "6 d 04:00") {
		t.Error("the router's uptime is shown outside the Firmware card")
	}
	if strings.Contains(got, "Protected") {
		t.Error("an answering plugin is not proof the firewall protects the network")
	}
}

func TestRenderOverviewFollowsTheLayout(t *testing.T) {
	o := fullOverview()
	o.Layout = ParseOverviewLayout("1.traffic-internet-firewall.ports-tunnels-hardware-firmware.firewall.resources")
	got := render(t, newRenderer(t), o)
	if want := "traffic internet firewall ports tunnels hardware firmware"; strings.Join(cardOrder(got), " ") != want {
		t.Fatalf("order = %v, want %s", cardOrder(got), want)
	}
	if strings.Contains(got, `data-verso-meter="sys-cpu"`) {
		t.Error("a hidden card must not be rendered")
	}
	if !strings.Contains(got, `data-overview-restore="resources"`) {
		t.Error("a hidden card must stand on the shelf")
	}
	firewall := got[strings.Index(got, `data-overview-card="firewall"`):]
	firewall = firewall[:strings.Index(firewall, "</article>")]
	for _, want := range []string{"data-closed", `aria-expanded="false"`, `aria-label="Open Firewall"`, `data-overview-card-summary aria-hidden="false"`} {
		if !strings.Contains(firewall, want) {
			t.Errorf("closed card missing %q", want)
		}
	}
	internet := got[strings.Index(got, `data-overview-card="internet"`):]
	internet = internet[:strings.Index(internet, "</article>")]
	if strings.Contains(internet, "data-closed") || !strings.Contains(internet, `aria-expanded="true"`) {
		t.Error("an open card must say so")
	}
}

func TestOverviewDegradesWithoutInventingState(t *testing.T) {
	r := newRenderer(t)
	bare := render(t, r, &Overview{})
	for _, want := range []string{"Your network at a glance", "Status unavailable", "Unavailable", "N/A", "CPU Temperature", "Fan speed", "Power draw", "System readings unavailable", "No address.", "No prefix was delegated."} {
		if !strings.Contains(bare, want) {
			t.Errorf("missing fallback %q", want)
		}
	}
	for _, falseClaim := range []string{"online", "All ports linked", "All up", "Connected"} {
		if strings.Contains(bare, ">"+falseClaim+"<") {
			t.Errorf("unknown router claims %q", falseClaim)
		}
	}
	down := render(t, r, &Overview{WANKnown: true, WANUp: false, InterfacesKnown: true})
	if !strings.Contains(down, "Your network is offline") || !strings.Contains(down, `data-overview-tone="danger"`) || !strings.Contains(down, "Not connected") {
		t.Error("offline WAN must change the verdict and the Internet card")
	}
	none := render(t, r, &Overview{InterfacesKnown: true})
	if !strings.Contains(none, "None observed") || !strings.Contains(none, "No tunnel is up.") {
		t.Error("no tunnels must be said in the Tunnels card")
	}
}

func TestRenderOverviewTrafficAutoScale(t *testing.T) {
	got := render(t, newRenderer(t), &Overview{DownSeries: []float64{80, 400, 120}, UpSeries: []float64{20, 40, 30}})
	for _, want := range []string{">460 Mbit/s<", ">345<", ">230<", ">115<", `viewBox="0 0 620 240"`, `data-chart-padding="0"`, "verso-traffic-card"} {
		if !strings.Contains(got, want) {
			t.Errorf("existing shared-scale renderer missing %q", want)
		}
	}
	if strings.Contains(got, ">Internet traffic</h2>") {
		t.Error("the card's header names the graph; the graph draws no title of its own")
	}
}

func TestOverviewVerdictFollowsReadings(t *testing.T) {
	o := &Overview{WANKnown: true, WANUp: true, InterfacesKnown: true, Interfaces: []OverviewInterface{{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}}, {Name: "wg0", Kind: "tunnel", State: "up", Networks: []string{"vpn"}}}}
	if s := o.LiveStatus(plain); s.Tone != "success" || s.Accent != "online" {
		t.Fatalf("up = %+v", s)
	}
	o.Interfaces[1].State = "down"
	if s := o.LiveStatus(plain); s.Tone != "warning" || s.Lead != "Your network needs attention" {
		t.Fatalf("tunnel down = %+v", s)
	}
	o.Interfaces[1].State = "up"
	o.SysMetrics = []OverviewMeter{{Label: "Memory", Band: "danger"}}
	if s := o.LiveStatus(plain); s.Tone != "warning" {
		t.Fatalf("resource at critical = %+v", s)
	}
	o.WANUp = false
	if s := o.LiveStatus(plain); s.Tone != "danger" || s.Lead != "Your network is offline" {
		t.Fatalf("WAN down = %+v", s)
	}
	o.WANKnown = false
	if s := o.LiveStatus(plain); s.Tone != "neutral" || s.Lead != "Your network at a glance" {
		t.Fatalf("read failed = %+v", s)
	}
}

// A spare port and the kernel's own tunnels are down, but nobody asked for
// them, so nothing needs attention.
func TestOverviewVerdictIgnoresUnusedInterfaces(t *testing.T) {
	o := &Overview{WANKnown: true, WANUp: true, InterfacesKnown: true, Interfaces: []OverviewInterface{
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}},
		{Name: "lan4", Physical: true, State: "down"},
		{Name: "sit0", Kind: "tunnel", State: "down"},
		{Name: "wg0", Kind: "tunnel", State: "up"},
	}}
	if s := o.LiveStatus(plain); s.Tone != "success" {
		t.Fatalf("verdict = %+v, want online", s)
	}
}

func TestOverviewLiveStatusCarriesEveryCard(t *testing.T) {
	o := fullOverview()
	o.Layout = ParseOverviewLayout("1.internet-firewall-ports-tunnels.traffic-hardware-firmware..resources")
	live := o.LiveStatus(plain)
	if len(live.Cards) != len(overviewCardIDs()) {
		t.Fatalf("live cards = %d, want every card, hidden ones included", len(live.Cards))
	}
	for i, spec := range overviewCardSpecs() {
		if live.Cards[i].ID != spec.ID || live.Cards[i].cardHead != spec.head(o, plain) {
			t.Errorf("live %s = %+v, want the first paint's head", spec.ID, live.Cards[i])
		}
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
		if got := interfaceCount(plain, tc.n); got != tc.en {
			t.Errorf("en(%d)=%q", tc.n, got)
		}
		if got := interfaceCount(tr, tc.n); got != tc.sl {
			t.Errorf("sl(%d)=%q", tc.n, got)
		}
	}
}

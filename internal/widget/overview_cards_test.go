// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"slices"
	"strings"
	"testing"
)

var plain = func(s string) string { return s }

// head reads one card's header off the registry, as the page and the stream do.
func head(t *testing.T, o *Overview, id string) cardHead {
	t.Helper()
	for _, spec := range overviewCardSpecs() {
		if spec.ID == id {
			return spec.head(o, plain)
		}
	}
	t.Fatalf("no card %q", id)
	return cardHead{}
}

func TestOverviewCardRegistryMatchesTheLayout(t *testing.T) {
	var ids []string
	for _, spec := range overviewCardSpecs() {
		ids = append(ids, spec.ID)
		if spec.Title == "" || spec.Icon == "" || spec.head == nil || spec.body == nil {
			t.Errorf("card %q is incomplete", spec.ID)
		}
	}
	if !slices.Equal(ids, overviewCardIDs()) {
		t.Fatalf("registry = %v, layout knows %v", ids, overviewCardIDs())
	}
}

func TestInternetCardHead(t *testing.T) {
	if h := head(t, &Overview{}, "internet"); h.Status != "Unavailable" || h.Tone != "neutral" {
		t.Errorf("unknown = %+v", h)
	}
	// Up, its line is the address the internet sees; down, the status says
	// it all.
	up := head(t, &Overview{WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min", WANAddress: "203.0.113.7/32"}, "internet")
	if up.Status != "Connected" || up.Tone != "success" || up.Summary != "203.0.113.7" {
		t.Errorf("up = %+v", up)
	}
	down := head(t, &Overview{WANKnown: true, WANDevice: "eth0", WANAddress: "203.0.113.7/32"}, "internet")
	if down.Status != "Not connected" || down.Tone != "danger" || down.Summary != "" {
		t.Errorf("down = %+v", down)
	}
}

// No card shows the firewall, but a ruleset the kernel has not fully loaded
// still makes the verdict read that something needs attention; an unread one
// claims nothing.
func TestFirewallStateReachesTheVerdict(t *testing.T) {
	for state, tone := range map[string]string{"": "success", "active": "success", "inactive": "warning", "partial": "warning"} {
		o := &Overview{WANKnown: true, WANUp: true, FirewallState: state}
		if v := o.LiveStatus(plain); v.Tone != tone {
			t.Errorf("firewall %q: verdict %s, want %s", state, v.Tone, tone)
		}
	}
}

// The kernel makes sit0 and ip6tnl0 on its own, and a spare port has no
// cable: down, but nobody asked for them, so nothing needs attention. A tunnel
// someone did bring up counts whether or not uci names its device.
func TestPortsAndTunnelsCardHeads(t *testing.T) {
	o := &Overview{InterfacesKnown: true, DevicesKnown: true, DevicesOnline: 9, Interfaces: []OverviewInterface{
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}},
		{Name: "lan1", Physical: true, State: "down", Relations: []OverviewInterfaceRelation{{Name: "br-lan"}}},
		{Name: "lan4", Physical: true, State: "down"},
		{Name: "sit0", Kind: "tunnel", State: "down"},
		{Name: "tailscale0", Kind: "tunnel", State: "up"},
		{Name: "wg0", Kind: "tunnel", State: "up"},
	}}
	ports := head(t, o, "ports")
	if ports.Status != "1 port without link" || ports.Tone != "warning" || ports.Summary != "9 devices connected" {
		t.Errorf("ports = %+v, want the spare lan4 not counted, and the devices as its line", ports)
	}
	tunnels := head(t, o, "tunnels")
	if tunnels.Status != "All up" || tunnels.Tone != "success" || tunnels.Summary != "2 tunnels" {
		t.Errorf("tunnels = %+v, want tailscale0 and wg0 counted, not the kernel's own", tunnels)
	}
	o.Interfaces[5].State = "down"
	if h := head(t, o, "tunnels"); h.Status != "1 of 2 down" || h.Tone != "danger" || h.Summary != "wg0" {
		t.Errorf("a tunnel down = %+v, want it named", h)
	}
	o.Interfaces[5].State = "unknown"
	if h := head(t, o, "tunnels"); h.Status != "State unknown" || h.Tone != "neutral" {
		t.Errorf("a tunnel's unknown kernel state is not a connected peer: %+v", h)
	}
	if h := head(t, &Overview{InterfacesKnown: true}, "tunnels"); h.Status != "None observed" || h.Tone != "neutral" {
		t.Errorf("no tunnels = %+v", h)
	}
	if h := head(t, &Overview{}, "ports"); h.Status != "Unavailable" {
		t.Errorf("unknown interfaces = %+v", h)
	}
	linked := head(t, &Overview{InterfacesKnown: true, Interfaces: []OverviewInterface{{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}}}}, "ports")
	if linked.Status != "All ports linked" || linked.Tone != "success" {
		t.Errorf("linked = %+v", linked)
	}
}

func TestReadingCardsShowAStatusOnlyOutOfBand(t *testing.T) {
	o := &Overview{
		DownNow: 84.2, UpNow: 1260, Firmware: "OpenWrt 25.12.4", Uptime: "3 d 4 h",
		Temperature: "52 °C · normal", TempDot: "success", Fan: "2400 rpm", Power: "7.8 W",
		SysMetrics: []OverviewMeter{
			{Name: "sys-load", Label: "Load", Value: "0.42", Band: "success"},
			{Name: "sys-cpu", Label: "CPU", Value: "12", Unit: "%", Band: "success"},
			{Name: "sys-memory", Label: "Memory", Value: "16", Unit: "%", Band: "success"},
		},
	}
	// Each closed card says one thing: the reading most worth a glance.
	// Resources always says memory, so its line never swaps one reading for
	// another as they pass each other.
	for id, summary := range map[string]string{
		"traffic":   "84 Mb/s down · 1.3 Gb/s up",
		"resources": "Memory 16 %",
		"hardware":  "52 °C",
		"firmware":  "OpenWrt 25.12.4",
	} {
		if h := head(t, o, id); h.Status != "" || h.Summary != summary {
			t.Errorf("%s = %+v, want no status and %q", id, h, summary)
		}
	}
	o.SysMetrics[2].Band = "warning"
	if h := head(t, o, "resources"); h.Status != "Memory is high" || h.Tone != "warning" {
		t.Errorf("memory in its warning band = %+v", h)
	}
	o.SysMetrics[1].Band = "danger"
	if h := head(t, o, "resources"); h.Status != "CPU is high" || h.Tone != "danger" {
		t.Errorf("the worst band names the card = %+v", h)
	}
	o.TempDot = "danger"
	if h := head(t, o, "hardware"); h.Status != "CPU temperature is high" || h.Tone != "danger" {
		t.Errorf("hot = %+v", h)
	}
	if h := head(t, &Overview{}, "hardware"); h.Summary != "" || h.Status != "" {
		t.Errorf("no sensors = %+v", h)
	}
}

// cardBody renders one card's body as the page does.
func cardBody(t *testing.T, o *Overview, id string) string {
	t.Helper()
	base := newRenderer(t)
	r := &Renderer{tmpl: base.setFor(""), md: base.md, seq: base.seq}
	for _, spec := range overviewCardSpecs() {
		if spec.ID == id {
			html, err := spec.body(o, r, "")
			if err != nil {
				t.Fatal(err)
			}
			return normalizeHTML(string(html))
		}
	}
	t.Fatalf("no card %q", id)
	return ""
}

// row is one listing row's markup, found by its live key.
func row(t *testing.T, body, key string) string {
	t.Helper()
	at := strings.Index(body, `data-verso-row="`+key+`"`)
	if at < 0 {
		t.Fatalf("no row %s in %s", key, body)
	}
	tail := body[at:]
	return tail[:strings.Index(tail, "</tr>")]
}

func TestPortsAndTunnelsCardBodies(t *testing.T) {
	o := &Overview{InterfacesKnown: true, Interfaces: []OverviewInterface{
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}, RxBPS: 18_400_000, TxBPS: 2_100_000},
		{Name: "eth2", Physical: true, State: "lowerlayerdown", Networks: []string{"lan"}},
		{Name: "eth4", Physical: true, State: "down"},
		{Name: "wg0", Kind: "tunnel", State: "down", Proto: "wireguard"},
		{Name: "br-lan", Kind: "bridge", State: "up"},
	}}
	ports := cardBody(t, o, "ports")
	if up := row(t, ports, "interface:eth0"); !strings.Contains(up, "wan") || !strings.Contains(up, "18 Mb/s") || !strings.Contains(up, "2.1 Mb/s") || !strings.Contains(up, "bg-green") || !strings.Contains(up, ">Up<") {
		t.Errorf("an up port = %s", up)
	}
	if lost := row(t, ports, "interface:eth2"); !strings.Contains(lost, ">Down<") || !strings.Contains(lost, "lan") {
		t.Errorf("a lost-link port = %s", lost)
	}
	// A spare port says its link as any port does, and moves nothing.
	if spare := row(t, ports, "interface:eth4"); !strings.Contains(spare, ">Down<") || strings.Contains(spare, "Mb/s") || strings.Contains(spare, `data-verso-cell="rx-rate"`) {
		t.Errorf("a spare port = %s", spare)
	}
	if strings.Contains(ports, "br-lan<") {
		t.Error("the Ports card lists physical ports only")
	}
	tunnels := cardBody(t, o, "tunnels")
	if down := row(t, tunnels, "interface:wg0"); !strings.Contains(down, "wireguard") || !strings.Contains(down, ">Down<") {
		t.Errorf("a down tunnel = %s", down)
	}
}

func TestInternetCardBodySaysWhatIsMissing(t *testing.T) {
	body := cardBody(t, &Overview{V4: []OverviewFact{{Label: "Address", Value: "192.0.2.1/24", Copy: true}}, V6: []OverviewFact{{Label: "Status", Value: "Not configured"}}}, "internet")
	if !strings.Contains(body, "192.0.2.1/24") || !strings.Contains(body, `x-data="copy"`) {
		t.Error("the IPv4 address must be listed and copyable")
	}
	if !strings.Contains(body, "No prefix was delegated.") || !strings.Contains(body, "border-dashed") || !strings.Contains(body, "border border-faint") {
		t.Errorf("an absent IPv6 needs the empty slot and its hollow packet: %s", body)
	}
}

// The uplink's device and how long it has been connected head the Internet
// card, the time kept live by the stream; a down uplink has no time to state.
func TestInternetCardBodyNamesTheUplink(t *testing.T) {
	up := cardBody(t, &Overview{WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min"}, "internet")
	for _, want := range []string{">Device<", ">pppoe-wan<", ">Connected for<", `data-verso-prop="wan-uptime"`, ">2 h 14 min<"} {
		if !strings.Contains(up, want) {
			t.Errorf("an up uplink's card missing %q: %s", want, up)
		}
	}
	if strings.Index(up, "pppoe-wan") > strings.Index(up, "IPv4") {
		t.Error("the uplink heads the card, over its address families")
	}
	down := cardBody(t, &Overview{WANKnown: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min"}, "internet")
	if !strings.Contains(down, ">pppoe-wan<") || strings.Contains(down, "Connected for") {
		t.Errorf("a down uplink names its device and states no time: %s", down)
	}
	if none := cardBody(t, &Overview{}, "internet"); strings.Contains(none, ">Device<") {
		t.Errorf("no uplink, no device row: %s", none)
	}
}

func TestOverviewCardHeadsNeverLeakPluralContext(t *testing.T) {
	o := &Overview{InterfacesKnown: true, DevicesKnown: true, DevicesOnline: 2, FirewallState: "active",
		Interfaces: []OverviewInterface{{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}}, {Name: "wg0", Kind: "tunnel", State: "up"}}}
	for _, spec := range overviewCardSpecs() {
		h := spec.head(o, plain)
		if strings.Contains(h.Summary+h.Status, "(two)") || strings.Contains(h.Summary+h.Status, "(few)") {
			t.Errorf("%s leaks plural context: %+v", spec.ID, h)
		}
	}
}

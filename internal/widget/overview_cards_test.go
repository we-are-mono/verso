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
	up := head(t, &Overview{WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: "2 h 14 min"}, "internet")
	if up.Status != "Connected" || up.Tone != "success" || up.Summary != "for 2 h 14 min · pppoe-wan" {
		t.Errorf("up = %+v", up)
	}
	down := head(t, &Overview{WANKnown: true, WANDevice: "eth0"}, "internet")
	if down.Status != "Not connected" || down.Tone != "danger" || down.Summary != "WAN is down · eth0" {
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
	if ports.Status != "1 port without link" || ports.Tone != "warning" || ports.Summary != "lan1 · 6 interfaces · 9 devices connected" {
		t.Errorf("ports = %+v, want lan1 named, not the spare lan4", ports)
	}
	tunnels := head(t, o, "tunnels")
	if tunnels.Status != "All up" || tunnels.Tone != "success" || tunnels.Summary != "tailscale0 · wg0" {
		t.Errorf("tunnels = %+v, want tailscale0 and wg0, not the kernel's own", tunnels)
	}
	o.Interfaces[5].State = "down"
	if h := head(t, o, "tunnels"); h.Status != "1 of 2 down" || h.Tone != "danger" {
		t.Errorf("a tunnel down = %+v", h)
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
		DownVal: "84.2", UpVal: "12.6", Firmware: "OpenWrt 25.12.4", Uptime: "3 d 4 h",
		Temperature: "52 °C · normal", TempDot: "success", Fan: "2400 rpm", Power: "7.8 W",
		SysMetrics: []OverviewMeter{
			{Label: "Load", Value: "0.42", Band: "success"},
			{Label: "CPU", Value: "12", Unit: "%", Band: "success"},
			{Label: "Memory", Value: "16", Unit: "%", Band: "success"},
		},
	}
	for id, summary := range map[string]string{
		"traffic":   "84.2 Mbit/s down · 12.6 Mbit/s up",
		"resources": "Load 0.42 · CPU 12 % · Memory 16 %",
		"hardware":  "52 °C · 2400 rpm · 7.8 W",
		"firmware":  "OpenWrt 25.12.4 · up 3 d 4 h",
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
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}, RxRate: "18.4 Mbps", TxRate: "2.1 Mbps"},
		{Name: "eth2", Physical: true, State: "lowerlayerdown", Networks: []string{"lan"}},
		{Name: "eth4", Physical: true, State: "down"},
		{Name: "wg0", Kind: "tunnel", State: "down", Proto: "wireguard"},
		{Name: "br-lan", Kind: "bridge", State: "up"},
	}}
	ports := cardBody(t, o, "ports")
	if up := row(t, ports, "interface:eth0"); !strings.Contains(up, "wan") || !strings.Contains(up, "18.4 Mbps") || !strings.Contains(up, "bg-green") || !strings.Contains(up, ">Up<") {
		t.Errorf("an up port = %s", up)
	}
	if lost := row(t, ports, "interface:eth2"); !strings.Contains(lost, ">Down<") || !strings.Contains(lost, "lan") {
		t.Errorf("a lost-link port = %s", lost)
	}
	if !strings.Contains(ports, "Not in use") || strings.Contains(ports, `data-verso-row="interface:eth4"`) {
		t.Error("a spare port reads Not in use and keeps out of the stream")
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

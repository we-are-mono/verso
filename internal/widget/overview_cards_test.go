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
	up := head(t, &Overview{WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: secs(8040), WANAddress: "203.0.113.7/32"}, "internet")
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
	for state, want := range map[string][2]string{
		"":         {"success", "Your network is "},
		"active":   {"success", "Your network is "},
		"inactive": {"danger", "The firewall is not running!"},
		"partial":  {"warning", "The firewall is only partly loaded."},
	} {
		o := &Overview{WANKnown: true, WANUp: true, FirewallState: state}
		if v := o.LiveStatus(plain); v.Tone != want[0] || v.Lead != want[1] {
			t.Errorf("firewall %q: verdict %s %q, want %s %q", state, v.Tone, v.Lead, want[0], want[1])
		}
	}
}

// The internet reaching the web interface is a cause no card shows, and it
// outranks most that one does: alone it is the verdict in its own words, and
// beside another it is counted.
func TestAnExposedWebInterfaceReachesTheVerdict(t *testing.T) {
	o := &Overview{WANKnown: true, WANUp: true, Exposed: true}
	if v := o.LiveStatus(plain); v.Tone != "warning" || v.Lead != "The internet can reach this web interface." {
		t.Errorf("verdict %s %q, want the exposure in its own words", v.Tone, v.Lead)
	}
	o.SysMetrics = []OverviewMeter{{Label: "Storage", Band: "warning"}}
	if v := o.LiveStatus(plain); v.Lead != "2 things need attention." {
		t.Errorf("verdict %q, want the exposure counted beside the storage", v.Lead)
	}
}

// TestEachCardSaysItsCause: a card out of band says what is wrong in a
// sentence that stands without the card, for the heading to quote; in band it
// says nothing.
func TestEachCardSaysItsCause(t *testing.T) {
	o := &Overview{InterfacesKnown: true, Interfaces: []OverviewInterface{
		{Name: "lan1", Physical: true, State: "down", Networks: []string{"lan"}},
		{Name: "wg0", Kind: "tunnel", State: "down"},
		{Name: "wg1", Kind: "tunnel", State: "up"},
	}, SysMetrics: []OverviewMeter{{Label: "Storage", Band: "warning"}}, Hardware: dkHardware()}
	o.Hardware.CPU.Tone = "warning"
	for card, cause := range map[string]string{
		"ports": "1 port without link", "tunnels": "Tunnel wg0 is down", "resources": "Storage is high", "hardware": "cluster is hot",
	} {
		if h := head(t, o, card); h.Cause != cause {
			t.Errorf("%s cause = %q, want %q (status %q)", card, h.Cause, cause, h.Status)
		}
	}
	if h := head(t, o, "tunnels"); h.Status != "1 of 2 down" {
		t.Errorf("the Tunnels header keeps its words in context: %q", h.Status)
	}
	o.Interfaces[2].State = "down"
	o.Interfaces = append(o.Interfaces, OverviewInterface{Name: "wg2", Kind: "tunnel", State: "up"})
	if h := head(t, o, "tunnels"); h.Cause != "2 of 3 tunnels down" {
		t.Errorf("several tunnels down = %q", h.Cause)
	}
	calm := &Overview{InterfacesKnown: true, SysMetrics: []OverviewMeter{{Label: "Storage"}}, Hardware: dkHardware()}
	for _, card := range []string{"ports", "tunnels", "resources", "hardware"} {
		if h := head(t, calm, card); h.Cause != "" {
			t.Errorf("%s in band says %q", card, h.Cause)
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
	// Every port in use linked is what the panel and the list under the header
	// already show, so the header says nothing then.
	linked := head(t, &Overview{InterfacesKnown: true, Interfaces: []OverviewInterface{{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}}}}, "ports")
	if linked.Status != "" || linked.Tone != "neutral" {
		t.Errorf("linked = %+v, want no status", linked)
	}
}

func TestReadingCardsShowAStatusOnlyOutOfBand(t *testing.T) {
	o := &Overview{
		DownNow: 84.2, UpNow: 1260, Firmware: "OpenWrt 25.12.4", Uptime: secs(273600),
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
		"traffic":   "84\U0000202fMb/s down · 1.3\U0000202fGb/s up",
		"resources": "Memory 16\U0000202f%",
		"system":    "OpenWrt 25.12.4",
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
	if up := row(t, ports, "interface:eth0"); !strings.Contains(up, "wan") || !strings.Contains(up, "18\U0000202fMb/s") || !strings.Contains(up, "2.1\U0000202fMb/s") || !strings.Contains(up, `data-verso-tone="success"`) || !strings.Contains(up, ">Up<") {
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

// TestPortsCardOpensWithThePanel: the router's panel heads the Ports card and
// the listing under it follows the panel's left-to-right order, so the card
// has one order; a DSA conduit is no port, so it has no row and its link
// counts for nothing.
func TestPortsCardOpensWithThePanel(t *testing.T) {
	o := &Overview{InterfacesKnown: true,
		Panel: &Ports{Items: []PortItem{
			{Kind: "rj45", Label: "eth1", Iface: "eth1"}, {Kind: "rj45", Label: "eth2", Iface: "eth2"},
			{Kind: "rj45", Label: "eth0", Iface: "eth0", Linked: true},
		}},
		Interfaces: []OverviewInterface{
			{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}},
			{Name: "eth1", Physical: true, State: "up", Networks: []string{"lan"}},
			{Name: "eth2", Physical: true, State: "up", Networks: []string{"lan"}},
			{Name: "eth9", Physical: true, Conduit: true, State: "down", Networks: []string{"lan"}},
		}}
	ports := cardBody(t, o, "ports")
	panel, eth1, eth2, eth0 := strings.Index(ports, `class="verso-panel"`),
		strings.Index(ports, `data-verso-row="interface:eth1"`), strings.Index(ports, `data-verso-row="interface:eth2"`), strings.Index(ports, `data-verso-row="interface:eth0"`)
	if panel < 0 || !(panel < eth1 && eth1 < eth2 && eth2 < eth0) {
		t.Errorf("want the panel, then eth1, eth2, eth0 (at %d, %d, %d, %d):\n%s", panel, eth1, eth2, eth0, ports)
	}
	if strings.Contains(ports, "interface:eth9") {
		t.Error("a DSA conduit is no port and has no row")
	}
	if head := o.portsHead(identityTranslator); head.Status != "" || head.Tone != "neutral" {
		t.Errorf("a conduit's link counts for nothing; head = %+v", head)
	}
}

func TestInternetCardBodySaysWhatIsMissing(t *testing.T) {
	body := cardBody(t, &Overview{V4: []OverviewFact{{Label: "Address", Value: "192.0.2.1/24", Copy: true}}, V6: []OverviewFact{{Label: "Status", Value: "Not configured"}}}, "internet")
	if !strings.Contains(body, "192.0.2.1/24") || !strings.Contains(body, `x-data="copy"`) {
		t.Error("the IPv4 address must be listed and copyable")
	}
	if !strings.Contains(body, "No prefix was delegated.") || !strings.Contains(body, "border-dashed") || !strings.Contains(body, `<span aria-hidden="true" class="verso-mark"></span>`) {
		t.Errorf("an absent IPv6 needs the empty slot and its hollow packet: %s", body)
	}
}

// The uplink's device and how long it has been connected head the Internet
// card, the time kept live by the stream; a down uplink has no time to state.
func TestInternetCardBodyNamesTheUplink(t *testing.T) {
	up := cardBody(t, &Overview{WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: secs(8040)}, "internet")
	for _, want := range []string{">Device<", ">pppoe-wan<", ">Connected for<", `data-verso-prop="wan-uptime"`, written("2", "h", "14", "min")} {
		if !strings.Contains(up, want) {
			t.Errorf("an up uplink's card missing %q: %s", want, up)
		}
	}
	if strings.Index(up, "pppoe-wan") > strings.Index(up, "IPv4") {
		t.Error("the uplink heads the card, over its address families")
	}
	down := cardBody(t, &Overview{WANKnown: true, WANDevice: "pppoe-wan", WANUptime: secs(8040)}, "internet")
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

// dkHardware is the DK's Hardware card as the server formats it, in band.
func dkHardware() *OverviewSensors {
	return &OverviewSensors{
		CPU:   &OverviewSensor{Key: "hw-cpu", Name: "cluster", Value: "48 °C"},
		Power: &OverviewSensor{Key: "hw-power", Name: "USB Power Delivery", Value: "9.8 W · 20.10 V · 0.49 A"},
		Fan:   &OverviewSensor{Key: "hw-fan", Name: "System Fan 1", Value: "2,622 rpm"},
	}
}

// The Hardware header speaks only out of band: a hot CPU before a stopped
// fan. Its line is always the CPU temperature.
func TestHardwareCardHead(t *testing.T) {
	cases := []struct {
		name                  string
		edit                  func(*OverviewSensors)
		tone, status, summary string
	}{
		{"in band", func(*OverviewSensors) {}, "neutral", "", "48 °C"},
		{"a warm CPU", func(s *OverviewSensors) { s.CPU.Tone = "warning" }, "warning", "cluster is hot", "48 °C"},
		{"the fan stops", func(s *OverviewSensors) { s.Fan.Tone = "warning" }, "warning", "System Fan 1 stopped", "48 °C"},
		{"a critical CPU beside a stopped fan", func(s *OverviewSensors) {
			s.Fan.Tone, s.CPU.Tone = "warning", "danger"
		}, "danger", "cluster is hot", "48 °C"},
		{"no CPU reading", func(s *OverviewSensors) { s.CPU = nil }, "neutral", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hw := dkHardware()
			c.edit(hw)
			if h := head(t, &Overview{Hardware: hw}, "hardware"); h.Tone != c.tone || h.Status != c.status || h.Summary != c.summary {
				t.Errorf("head = %+v, want %s %q %q", h, c.tone, c.status, c.summary)
			}
		})
	}
	for _, none := range []*OverviewSensors{nil, {}} {
		if h := head(t, &Overview{Hardware: none}, "hardware"); h.Tone != "neutral" || h.Status != "Unavailable" {
			t.Errorf("no sensor answered: head = %+v", h)
		}
	}
	// A box with no reading still says what needs attention.
	alone := &OverviewSensors{Alerts: []OverviewAlert{{Tone: "warning", Words: "Storage is wearing out"}}}
	if h := head(t, &Overview{Hardware: alone}, "hardware"); h.Tone != "warning" || h.Status != "Storage is wearing out" {
		t.Errorf("an alert with no reading: head = %+v", h)
	}
}

// The hardware's verdict is one grading the Overview card's header and the
// Hardware page's heading both speak: a hot CPU before a stopped fan, and
// nothing while both are in band.
func TestHardwareVerdict(t *testing.T) {
	identity := func(s string) string { return s }
	cases := []struct {
		name        string
		edit        func(*OverviewSensors)
		tone, words string
	}{
		{"in band", func(*OverviewSensors) {}, "", ""},
		{"a warm CPU", func(s *OverviewSensors) { s.CPU.Tone = "warning" }, "warning", "cluster is hot"},
		{"a critical CPU", func(s *OverviewSensors) { s.CPU.Tone = "danger" }, "danger", "cluster is hot"},
		{"the fan stops", func(s *OverviewSensors) { s.Fan.Tone = "warning" }, "warning", "System Fan 1 stopped"},
		{"a hot CPU beside a stopped fan", func(s *OverviewSensors) {
			s.Fan.Tone, s.CPU.Tone = "warning", "warning"
		}, "warning", "cluster is hot"},
		{"no CPU, a stopped fan", func(s *OverviewSensors) { s.CPU, s.Fan.Tone = nil, "warning" }, "warning", "System Fan 1 stopped"},
		// What needs attention beyond the readings follows them in its tone,
		// and a danger anywhere speaks before any warning.
		{"a wearing disk", func(s *OverviewSensors) {
			s.Alerts = []OverviewAlert{{Tone: "warning", Words: "Storage is wearing out"}}
		}, "warning", "Storage is wearing out"},
		{"a warm CPU beside a wearing disk", func(s *OverviewSensors) {
			s.CPU.Tone = "warning"
			s.Alerts = []OverviewAlert{{Tone: "warning", Words: "Storage is wearing out"}}
		}, "warning", "cluster is hot"},
		{"a warm CPU beside a worn-out disk", func(s *OverviewSensors) {
			s.CPU.Tone = "warning"
			s.Alerts = []OverviewAlert{{Tone: "danger", Words: "Storage is worn out"}}
		}, "danger", "Storage is worn out"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hw := dkHardware()
			c.edit(hw)
			if tone, words := hw.Verdict(identity); tone != c.tone || words != c.words {
				t.Errorf("verdict = %q %q, want %q %q", tone, words, c.tone, c.words)
			}
		})
	}
	var none *OverviewSensors
	if tone, words := none.Verdict(identity); tone != "" || words != "" {
		t.Errorf("no sensors: verdict = %q %q", tone, words)
	}
}

// The Hardware card is the CPU, the main power rail and the main fan, one
// row each under the profile's names, their values on the right edge so a
// long name keeps its line; every row is keyed for the stream, a reading the
// board lacks has no row, and a card with none says so.
func TestHardwareCardBody(t *testing.T) {
	body := cardBody(t, &Overview{Hardware: dkHardware()}, "hardware")
	cpu, power, fan := strings.Index(body, `data-verso-prop="hw-cpu"`), strings.Index(body, `data-verso-prop="hw-power"`), strings.Index(body, `data-verso-prop="hw-fan"`)
	if cpu < 0 || !(cpu < power && power < fan) {
		t.Errorf("want CPU, power, fan in order (at %d, %d, %d):\n%s", cpu, power, fan, body)
	}
	for _, want := range []string{">USB Power Delivery<", ">9.8 W · 20.10 V · 0.49 A<", ">2,622 rpm<"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "data-verso-properties-head") || strings.Contains(body, "sm:w-24") {
		t.Errorf("one fact list, no heads, values on the right edge:\n%s", body)
	}
	noFan := dkHardware()
	noFan.Fan = nil
	if body := cardBody(t, &Overview{Hardware: noFan}, "hardware"); strings.Contains(body, "hw-fan") {
		t.Errorf("a board with no main fan has no fan row:\n%s", body)
	}
	if body := cardBody(t, &Overview{}, "hardware"); !strings.Contains(body, "No sensor answered.") {
		t.Errorf("no reading, and a sentence that says so:\n%s", body)
	}
}

// A board past its own limits is a cause like any other: the heading names
// what is wrong, not only what is wrong with the network.
func TestAHotBoardLeadsTheHeading(t *testing.T) {
	hw := dkHardware()
	hw.CPU.Tone = "danger"
	if v := (&Overview{WANKnown: true, WANUp: true, Hardware: hw}).verdict(plain); v.Tone != "danger" || v.Lead != "cluster is hot!" {
		t.Errorf("verdict = %+v, want the hot CPU", v)
	}
}

// A service procd stopped restarting is a cause the System card owns: its
// header names it (one by name, several counted), the heading may quote it,
// its facts list it, and only then does the card open the Services page.
func TestAFailedServiceIsTheSystemCardsCause(t *testing.T) {
	var system overviewCardSpec
	for _, spec := range overviewCardSpecs() {
		if spec.ID == "system" {
			system = spec
		}
	}
	o := &Overview{WANKnown: true, WANUp: true, Firmware: "OpenWrt 25.12.4"}
	if h := system.head(o, plain); h.Tone != "neutral" || h.Cause != "" || system.href(o) != "" {
		t.Fatalf("calm System card = %+v, door %q", h, system.href(o))
	}
	o.FailedServices = []string{"sysntpd"}
	if h := system.head(o, plain); h.Tone != "danger" || h.Status != "sysntpd failed" || h.Cause != h.Status || h.Summary != "OpenWrt 25.12.4" {
		t.Errorf("one failed = %+v", h)
	}
	if v := o.verdict(plain); v.Tone != "danger" || v.Lead != "sysntpd failed!" {
		t.Errorf("verdict = %+v, want the failed service", v)
	}
	if got := system.href(o); got != "/system/services" {
		t.Errorf("door = %q, want the Services page", got)
	}
	if body := cardBody(t, o, "system"); !strings.Contains(body, "Failed services") || !strings.Contains(body, "sysntpd") {
		t.Errorf("the facts list the failed service:\n%s", body)
	}
	o.FailedServices = []string{"odhcpd", "sysntpd"}
	if h := system.head(o, plain); h.Status != "2 services failed" {
		t.Errorf("two failed = %+v", h)
	}
}

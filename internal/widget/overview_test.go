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
		WANKnown: true, WANUp: true, WANDevice: "pppoe-wan", WANUptime: secs(8040),
		InterfacesKnown: true, DevicesKnown: true, DevicesOnline: 9,
		Model: "Mono Gateway Development Kit",
		Arch:  "aarch64_generic", Target: "layerscape/armv8_64b",
		TunnelsHref: "/plugins/wireguard/", InterfacesHref: "/plugins/interfaces/",
		Firmware: "OpenWrt 25.12.4", Kernel: "Linux 6.12.101", Uptime: secs(532800),
		Clock: "14:52:07", Zone: "CEST",
		FirewallState: "active",
		V4Proto:       "DHCP", V4: []OverviewFact{{Label: "Address", Value: "172.30.1.171/24", Copy: true}},
		V6Proto: "DHCPv6 client", V6: []OverviewFact{{Label: "Prefix", Value: "fd42:7ea:aa00::/56", Copy: true}, {Label: "Expires", Duration: secs(480)}},
		SysMetrics: []OverviewMeter{{Name: "sys-cpu", Label: "CPU", Icon: "cpu", Value: "27", Unit: "%", Fill: 27, Band: "success"}},
		Interfaces: []OverviewInterface{
			{Name: "eth0", Kind: "port", State: "up", Physical: true, Networks: []string{"wan"}, RxBPS: 18_400_000, TxBPS: 2_100_000},
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

// TestAnOverviewCardsSeamIsItsFramesInk: the line between a card's header
// and its body is drawn in the frame's own ink, as a page card's is, so the
// header reads as the card's and the lines inside it as its contents'. The
// body is the card block's, which draws that seam.
func TestAnOverviewCardsSeamIsItsFramesInk(t *testing.T) {
	got := render(t, newRenderer(t), fullOverview())
	if n := strings.Count(got, `<div class="verso-card__body verso-rhythm verso-overview__content">`); n == 0 || n != strings.Count(got, "verso-overview__content") {
		t.Errorf("an Overview card's body is not the card's in %d of its cards:\n%s", strings.Count(got, "verso-overview__content")-n, got)
	}
	declares(t, "card", ".verso-card__body", "border: 0 solid var(--color-rule-strong);", "border-top-width: 1px;")
	for _, rule := range []string{".verso-overview__content", ".verso-overview__content:where(.verso-overview__card[data-closed] *)"} {
		if body := cssRule(blockCSS(t, "overview"), rule); strings.Contains(body, "border") {
			t.Errorf("blocks/overview.css draws a seam of its own on %s:\n%s", rule, body)
		}
	}
}

func TestRenderOverviewCards(t *testing.T) {
	got := render(t, newRenderer(t), fullOverview())
	for _, want := range []string{
		`data-overview`, `data-overview-tone="success"`, "Your network is", `data-overview-accent class="verso-overview__accent">online`, "14:52:07", "CEST",
		`aria-label="Arrange cards"`, ">Arrange cards<", ">Done<", "Reset layout",
		// Internet
		"Connected", "IPv4", "dhcp", "172.30.1.171/24", "IPv6", "dhcpv6", "fd42:7ea:aa00::/56", `x-data="copy"`, written("8", "min"),
		// Ports and Tunnels
		`data-verso-row="interface:eth0"`, "18\U0000202fMb/s", `data-verso-row="interface:eth4"`, "wg-home", "wireguard", `href="/plugins/wireguard/"`,
		// Traffic, Resources, System
		"Mb/s down", `class="verso-chart__area"`, `data-verso-meter="sys-cpu"`, "Mono Gateway Development Kit",
		"OpenWrt 25.12.4", "Linux 6.12.101", `data-verso-prop="uptime"`, written("6", "d", "4", "h"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if want := []string{"internet", "ports", "tunnels", "traffic", "resources", "system", "hardware"}; strings.Join(cardOrder(got), " ") != strings.Join(want, " ") {
		t.Errorf("default order = %v, want %v", cardOrder(got), want)
	}
	for _, gone := range []string{"Internet is working", "Live overview", "data-overview-tile", "Uptime</span>", "(two)", "(few)", "CPU Temperature", "Fan speed", "Power draw"} {
		if strings.Contains(got, gone) {
			t.Errorf("overview still draws %q", gone)
		}
	}
	system := got[strings.Index(got, `data-overview-card="system"`):]
	system = system[:strings.Index(system, "</article>")]
	if strings.Count(got, written("6", "d", "4", "h")) != strings.Count(system, written("6", "d", "4", "h")) {
		t.Error("the router's uptime is shown outside the System card")
	}
	// The System card reads from the hardware up: the board, what it runs as,
	// then the firmware on it, how long it has been running, and its clock.
	labels := regexp.MustCompile(`<dt[^>]*>([^<]+)</dt>`).FindAllStringSubmatch(system, -1)
	var order []string
	for _, m := range labels {
		order = append(order, m[1])
	}
	if want := "Board, Architecture, Target platform, Firmware, Kernel, Uptime, Local time"; strings.Join(order, ", ") != want {
		t.Errorf("System rows = %v, want %s", order, want)
	}
	// The local time carries its zone and stays live.
	for _, want := range []string{"aarch64_generic", "layerscape/armv8_64b", `data-verso-prop="local-time"`, ">14:52:07 CEST<"} {
		if !strings.Contains(system, want) {
			t.Errorf("System card missing %q", want)
		}
	}
	if strings.Contains(got, "data-overview-clock") || strings.Count(got, "14:52:07") != 1 {
		t.Error("the clock stands once, in the System card, not beside the verdict")
	}
	if strings.Contains(got, "Firewall") {
		t.Error("the Overview has no Firewall card")
	}
	// The verdict's accent is the green's deep step.
	declares(t, "overview", ".verso-overview__accent", "color: var(--color-green-deep);")
}

func TestRenderOverviewFollowsTheLayout(t *testing.T) {
	o := fullOverview()
	o.Layout = ParseOverviewLayout("1.traffic-internet-ports.tunnels-system.ports.resources").Without("hardware")
	got := render(t, newRenderer(t), o)
	if want := "traffic internet ports tunnels system"; strings.Join(cardOrder(got), " ") != want {
		t.Fatalf("order = %v, want %s", cardOrder(got), want)
	}
	if strings.Contains(got, `data-verso-meter="sys-cpu"`) {
		t.Error("a hidden card must not be rendered")
	}
	if !strings.Contains(got, `data-overview-restore="resources"`) {
		t.Error("a hidden card must stand on the shelf")
	}
	ports := got[strings.Index(got, `data-overview-card="ports"`):]
	ports = ports[:strings.Index(ports, "</article>")]
	for _, want := range []string{"data-closed", `aria-expanded="false"`, `aria-label="Open Ports"`, `data-overview-card-summary aria-hidden="false"`} {
		if !strings.Contains(ports, want) {
			t.Errorf("closed card missing %q", want)
		}
	}
	internet := got[strings.Index(got, `data-overview-card="internet"`):]
	internet = internet[:strings.Index(internet, "</article>")]
	if strings.Contains(internet, "data-closed") || !strings.Contains(internet, `aria-expanded="true"`) {
		t.Error("an open card must say so")
	}
}

// TestOverviewShelfHoldsTheChipAHideClones: the page hides a card where it
// stands, so the shelf carries a blank chip to clone and the word it says once
// it holds one.
func TestOverviewShelfHoldsTheChipAHideClones(t *testing.T) {
	got := render(t, newRenderer(t), fullOverview())
	if !strings.Contains(got, `data-overview-shelf-label data-hidden-label="Hidden"`) || !strings.Contains(got, "No hidden cards") {
		t.Error("an empty shelf says so, and carries the word it says once a card is hidden")
	}
	start := strings.Index(got, "<template data-overview-chip>")
	if start < 0 {
		t.Fatal("the shelf must carry a blank chip")
	}
	chip := got[start:]
	chip = chip[:strings.Index(chip, "</template>")]
	for _, want := range []string{`data-overview-restore=""`, `class="verso-overview__restore"`, `class="verso-mark"`, "<svg", "data-overview-chip-title></span>"} {
		if !strings.Contains(chip, want) {
			t.Errorf("blank chip missing %q", want)
		}
	}
}

func TestOverviewDegradesWithoutInventingState(t *testing.T) {
	r := newRenderer(t)
	bare := render(t, r, &Overview{})
	for _, want := range []string{"Your network at a glance", "Unavailable", "System readings unavailable", "No address.", "No prefix was delegated."} {
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
	// The download lane scales to its own peak (400 under 600), the upload lane
	// to its own (40 under 60).
	for _, want := range []string{">600<", ">450<", ">300<", ">150<", ">60<", "data-verso-chart-unit>Mb/s<", `viewBox="0 0 620 96"`, `data-chart-padding="0"`, `id="overview-traffic-graph"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the Traffic card's graph missing %q", want)
		}
	}
	if strings.Contains(got, ">Internet traffic</h2>") {
		t.Error("the card's header names the graph; the graph draws no title of its own")
	}
	// In the card the plot is ruled in 24px squares, its values in a gutter on
	// its right and its window under it, none of them laid over the curve.
	for _, want := range []string{"data-verso-chart-gutter", `data-verso-place="compact"`, "data-verso-chart-window"} {
		if !strings.Contains(got, want) {
			t.Errorf("the card's chart layout missing %q", want)
		}
	}
	declares(t, "chart", `.verso-chart[data-verso-place="compact"]`, "--verso-chart-cell: 1.5rem;")
	if strings.Contains(got, "text-[2rem]") || strings.Contains(got, "bg-ground/85") {
		t.Error("the card's figures stand at reading size and its labels lie over nothing")
	}
}

// TestTheHeadingNamesTheCause: the heading is the most important true
// sentence on the page — offline over everything, the one thing wrong in its
// own words and tone, the count of several, online when nothing is.
func TestTheHeadingNamesTheCause(t *testing.T) {
	o := &Overview{WANKnown: true, WANUp: true, InterfacesKnown: true, Interfaces: []OverviewInterface{
		{Name: "eth0", Physical: true, State: "up", Networks: []string{"wan"}},
		{Name: "lan1", Physical: true, State: "up", Networks: []string{"lan"}},
		{Name: "wg0", Kind: "tunnel", State: "up", Networks: []string{"vpn"}},
	}}
	said := func() (string, string) { s := o.LiveStatus(plain); return s.Lead, s.Tone }
	// The verdict is a sentence, ending after its accent.
	if s := o.LiveStatus(plain); s.Tone != "success" || s.Accent != "online" || s.Tail != "." {
		t.Fatalf("up = %+v", s)
	}
	o.Interfaces[2].State = "down"
	if lead, tone := said(); lead != "Tunnel wg0 is down!" || tone != "danger" {
		t.Errorf("one tunnel down = %q %s", lead, tone)
	}
	o.Interfaces[2].State = "up"
	o.SysMetrics = []OverviewMeter{{Label: "Storage", Band: "warning"}}
	if lead, tone := said(); lead != "Storage is high." || tone != "warning" {
		t.Errorf("storage high = %q %s", lead, tone)
	}
	o.Interfaces[1].State = "down"
	if lead, tone := said(); lead != "2 things need attention." || tone != "warning" {
		t.Errorf("two causes = %q %s", lead, tone)
	}
	o.Interfaces[2].State = "down"
	if lead, tone := said(); lead != "3 things need attention!" || tone != "danger" {
		t.Errorf("a danger among three = %q %s", lead, tone)
	}
	o.WANUp = false
	if lead, tone := said(); lead != "Your network is offline!" || tone != "danger" {
		t.Errorf("WAN down = %q %s", lead, tone)
	}
	o.WANKnown, o.Interfaces, o.SysMetrics = false, nil, nil
	if lead, tone := said(); lead != "Your network at a glance." || tone != "neutral" {
		t.Errorf("nothing known = %q %s", lead, tone)
	}
}

// TestAHiddenCardsCauseStillLeads: hiding a card never hides its cause.
func TestAHiddenCardsCauseStillLeads(t *testing.T) {
	o := &Overview{WANKnown: true, WANUp: true, SysMetrics: []OverviewMeter{{Label: "Memory", Band: "warning"}}}
	o.Layout = ParseOverviewLayout("1.internet-ports-tunnels.traffic-system..resources")
	if s := o.LiveStatus(plain); s.Lead != "Memory is high." {
		t.Errorf("hidden Resources = %+v", s)
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
	o.Layout = ParseOverviewLayout("1.internet-ports-tunnels.traffic-system..resources")
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

// TestTheTrafficCardsWindowRidesItsHeader: the window governs the whole card,
// so it is the compact dropdown in the card's header, before the open-close
// control and hidden while arranging, fetching the window into the graph's
// place; no other card carries a control, and the graph carries none of its
// own. With no history behind the graph there is no window to offer.
func TestTheTrafficCardsWindowRidesItsHeader(t *testing.T) {
	got := render(t, newRenderer(t), &Overview{TrafficRangeURL: "/overview/traffic"})
	head := between(got, `data-overview-card="traffic"`, `</header>`)
	for _, want := range []string{
		`<span data-overview-card-control class="verso-overview__control">`,
		`<select name="range"`, `hx-get="/overview/traffic"`, `hx-target="#overview-traffic-graph"`, `hx-swap="outerHTML"`,
		`<option value="minute" selected>Last minute</option>`, `<option value="day">Last 24 hours</option>`,
		`<option value="week">Last 7 days</option>`, `<option value="month">Last 30 days</option>`, `<option value="year">Last year</option>`,
		`<div class="verso-control shrink-0" data-verso-size="compact">`,
		`class="verso-input" data-verso-size="compact">`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("the Traffic card's header missing %q:\n%s", want, head)
		}
	}
	// Arrange mode hides it with the acts.
	declares(t, "overview", ":is(.verso-overview__control, .verso-overview__act):is(.verso-overview[data-arranging] *)", "display: none;")
	// the compact dropdown rides the header's line of words: 24px, no shadow
	declares(t, "input", `.verso-input[data-verso-size="compact"]`, "height: calc(var(--spacing) * 6);", "box-shadow: none;")
	if strings.Index(head, "data-overview-card-control") > strings.Index(head, "data-overview-toggle") {
		t.Error("the window stands after the open-close control")
	}
	if strings.Count(got, "data-overview-card-control") != 1 || strings.Count(got, `name="range"`) != 1 {
		t.Error("a control stands in more than the Traffic card's header")
	}
	if bare := render(t, newRenderer(t), &Overview{}); strings.Contains(bare, "data-overview-card-control") {
		t.Error("with no history behind it, the Traffic card offers a window")
	}
}

func TestOverviewCountGrammarAndFallback(t *testing.T) {
	catalog := map[string]string{"%d tunnel": "%d tunel", "%d tunnels (two)": "%d tunela", "%d tunnels (few)": "%d tuneli", "%d tunnels": "%d tunelov"}
	tr := func(key string) string {
		if v, ok := catalog[key]; ok {
			return v
		}
		return key
	}
	for _, tc := range []struct {
		n      int
		en, sl string
	}{{0, "0 tunnels", "0 tunelov"}, {1, "1 tunnel", "1 tunel"}, {2, "2 tunnels", "2 tunela"}, {3, "3 tunnels", "3 tuneli"}, {4, "4 tunnels", "4 tuneli"}, {5, "5 tunnels", "5 tunelov"}, {101, "101 tunnels", "101 tunel"}, {102, "102 tunnels", "102 tunela"}} {
		if got := tunnelCount(plain, tc.n); got != tc.en {
			t.Errorf("en(%d)=%q", tc.n, got)
		}
		if got := tunnelCount(tr, tc.n); got != tc.sl {
			t.Errorf("sl(%d)=%q", tc.n, got)
		}
	}
}

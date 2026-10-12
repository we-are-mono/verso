// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/we-are-mono/verso/internal/units"
)

// cardHead is what a card's header says (ADR-022): its status word in a tone,
// and the one line a closed card shows after its title. A card of readings
// carries no status while its readings are in band. The page's first paint and
// the live stream read the same head, so the browser changes text, not policy.
type cardHead struct {
	Tone    string `json:"tone"` // "neutral" | "success" | "warning" | "danger"
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// overviewCardSpec is one card the Overview has: its identity, its header, its
// body, and the page it opens. The registry is the shell's; a card's content,
// status and summary change in its own functions and nowhere else.
type overviewCardSpec struct {
	ID, Title, Icon string
	// Opens names the page the card's Go link leads to; href resolves it from
	// the live registry, empty when no running plugin serves it.
	Opens string
	href  func(*Overview) string
	head  func(*Overview, func(string) string) cardHead
	body  func(*Overview, *Renderer, string) (template.HTML, error)
	// control is what the header carries for the body beside the status
	// (Traffic's window); nil carries nothing.
	control func(*Overview, *Renderer) (template.HTML, error)
}

// overviewCardSpecs are the Overview's cards, in the default layout's order.
func overviewCardSpecs() []overviewCardSpec {
	return []overviewCardSpec{
		{ID: "internet", Title: "Internet", Icon: "globe", Opens: "Interfaces", href: func(o *Overview) string { return o.InterfacesHref }, head: (*Overview).internetHead, body: (*Overview).internetBody},
		{ID: "ports", Title: "Ports", Icon: "ethernet-port", Opens: "Interfaces", href: func(o *Overview) string { return o.InterfacesHref }, head: (*Overview).portsHead, body: (*Overview).portsBody},
		{ID: "tunnels", Title: "Tunnels", Icon: "lock", Opens: "VPN", href: (*Overview).tunnelsHref, head: (*Overview).tunnelsHead, body: (*Overview).tunnelsBody},
		{ID: "traffic", Title: "Traffic", Icon: "activity", head: (*Overview).trafficHead, body: (*Overview).trafficBody, control: (*Overview).trafficControl},
		{ID: "resources", Title: "Resources", Icon: "gauge", head: (*Overview).resourcesHead, body: (*Overview).resourcesBody},
		{ID: "hardware", Title: "Hardware", Icon: "thermometer", head: (*Overview).hardwareHead, body: (*Overview).hardwareBody},
		{ID: "firmware", Title: "Firmware", Icon: "package", head: (*Overview).firmwareHead, body: (*Overview).firmwareBody},
	}
}

// joinSummary is a closed card's line: its parts that have something to say.
func joinSummary(parts ...string) string {
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// ── Internet ──

// internetHead is the uplink's actual state, how long it has held and the
// device it rides.
func (o *Overview) internetHead(tr func(string) string) cardHead {
	if !o.WANKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	if !o.WANUp {
		return cardHead{Tone: "danger", Status: tr("Not connected"), Summary: joinSummary(tr("WAN is down"), o.WANDevice)}
	}
	uptime := ""
	if o.WANUptime != "" {
		uptime = fmt.Sprintf(tr("for %s"), o.WANUptime)
	}
	return cardHead{Tone: "success", Status: tr("Connected"), Summary: joinSummary(uptime, o.WANDevice)}
}

// overviewFamilyView is one address family in the Internet card: its name and
// protocol over its facts, or the sentence that stands where they would.
type overviewFamilyView struct {
	Kind, Proto, Empty string
	Rows               template.HTML
}

// internetBody is the IPv4 and IPv6 facts, each address mono and copyable —
// the reading order for values a person compares line by line.
func (o *Overview) internetBody(r *Renderer, csrf string) (template.HTML, error) {
	families := []struct {
		kind, proto, empty string
		facts              []OverviewFact
	}{
		{"IPv4", o.V4Proto, "No address.", o.V4},
		{"IPv6", o.V6Proto, "No prefix was delegated.", o.V6},
	}
	views := make([]overviewFamilyView, 0, len(families))
	for _, f := range families {
		v := overviewFamilyView{Kind: f.kind, Proto: overviewProtocol(f.proto)}
		if len(f.facts) == 0 || (len(f.facts) == 1 && f.facts[0].Value == "Not configured") {
			v.Empty = r.tr(f.empty)
			views = append(views, v)
			continue
		}
		items := make([]Property, 0, len(f.facts))
		for _, fact := range f.facts {
			items = append(items, Property{Label: fact.Label, Value: fact.Value, Mono: fact.Copy, Emphasis: true, Copy: fact.Copy, Verbatim: fact.Label == "Expires"})
		}
		rows, err := o.renderWidget(r, &Properties{Style: "overview-connection", Align: "left", Items: items}, csrf)
		if err != nil {
			return "", err
		}
		v.Rows = rows
		views = append(views, v)
	}
	return r.fragment("overview.internet", views)
}

// ── Ports ──

// portsHead says whether every port something asked for has its cable, and
// how much hangs off them. A spare port's missing cable is nobody's concern.
func (o *Overview) portsHead(tr func(string) string) cardHead {
	if !o.InterfacesKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	caption := interfaceCount(tr, len(o.Interfaces))
	if o.DevicesKnown {
		caption += " · " + deviceCount(tr, o.DevicesOnline)
	}
	ports, down, unknown, first := 0, 0, 0, ""
	for _, n := range o.Interfaces {
		if !n.Physical || !n.inUse() {
			continue
		}
		ports++
		switch n.State {
		case "up":
		case "down", "lowerlayerdown":
			down++
			if first == "" {
				first = n.Name
			}
		default:
			unknown++
		}
	}
	h := cardHead{Tone: "neutral", Summary: joinSummary(first, caption)}
	switch {
	case down > 0:
		h.Tone = "warning"
		h.Status = overviewCount(tr, down, "%d port without link", "%d ports without link (two)", "%d ports without link (few)", "%d ports without link")
	case unknown > 0:
		h.Status = tr("State unknown")
	case ports > 0:
		h.Tone, h.Status = "success", tr("All ports linked")
	default:
		h.Status = tr("Virtual interfaces")
	}
	return h
}

// interfaceRow is one port or tunnel in a card's listing. A live row keys
// itself to the interface stream, which keeps its rates and state current.
func interfaceRow(nums units.Numbers, n OverviewInterface, kind TableCell) TableRow {
	return TableRow{ID: n.Name, Key: "interface:" + n.Name, Cells: []TableCell{
		{Text: n.Name, Verbatim: true},
		kind,
		{Text: nums.Rate(float64(n.RxBPS)).String(), Key: "rx-rate", Verbatim: true},
		{Text: nums.Rate(float64(n.TxBPS)).String(), Key: "tx-rate", Verbatim: true},
		stateCell(n),
	}}
}

// stateCell is an interface's link as the interface stream words and tones it.
func stateCell(n OverviewInterface) TableCell {
	variant := "neutral"
	if n.State == "up" {
		variant = "success"
	}
	return TableCell{Text: normalState(n.State), Variant: variant, Key: "state"}
}

// normalState is a kernel operstate as the interface stream words it.
func normalState(state string) string {
	switch state {
	case "":
		return "Unknown"
	case "lowerlayerdown":
		return "Down"
	}
	return strings.ToUpper(state[:1]) + state[1:]
}

// portsBody lists the physical ports: the network each carries, what moves on
// it now, and its link, status last.
func (o *Overview) portsBody(r *Renderer, csrf string) (template.HTML, error) {
	t := &Table{Columns: []TableColumn{
		{Label: "Port", Kind: "mono"}, {Label: "Network", Kind: "entity"},
		{Label: "Receive", Kind: "rate"}, {Label: "Transmit", Kind: "rate"}, {Label: "Status", Kind: "status"},
	}, EmptyText: "No physical ports were found."}
	for _, n := range o.Interfaces {
		if !n.Physical {
			continue
		}
		// A spare port carries no network and moves nothing, so it shows no
		// rate; its link stays live like any port's.
		if !n.inUse() {
			t.Rows = append(t.Rows, TableRow{ID: n.Name, Key: "interface:" + n.Name, Muted: true, Cells: []TableCell{
				{Text: n.Name, Verbatim: true}, {}, {}, {}, stateCell(n),
			}})
			continue
		}
		chips := make([]TableChip, 0, len(n.Networks))
		for _, network := range n.Networks {
			chips = append(chips, TableChip{Label: network, Icon: "network"})
		}
		for _, rel := range n.Relations {
			if len(n.Networks) == 0 {
				chips = append(chips, TableChip{Label: rel.Name, Icon: "network"})
			}
		}
		t.Rows = append(t.Rows, interfaceRow(r.numbers(), n, TableCell{Chips: chips}))
	}
	return o.renderWidget(r, t, csrf)
}

// ── Tunnels ──

func (o *Overview) tunnelsHref() string {
	if o.TunnelsHref != "" {
		return o.TunnelsHref
	}
	return o.InterfacesHref
}

// tunnelsHead reads each tunnel's netdev, which says nothing about a remote
// peer or a handshake: up is the device, not the far end.
func (o *Overview) tunnelsHead(tr func(string) string) cardHead {
	if !o.InterfacesKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	var names []string
	up, down := 0, 0
	for _, n := range o.Interfaces {
		if !n.tunnel() {
			continue
		}
		names = append(names, n.Name)
		switch n.State {
		case "up":
			up++
		case "down", "lowerlayerdown":
			down++
		}
	}
	h := cardHead{Tone: "neutral", Summary: joinSummary(names...)}
	switch {
	case len(names) == 0:
		h.Status = tr("None observed")
	case down > 0:
		h.Tone, h.Status = "danger", fmt.Sprintf(tr("%d of %d down"), down, len(names))
	case up == len(names):
		h.Tone, h.Status = "success", tr("All up")
	default:
		h.Status = tr("State unknown")
	}
	return h
}

// tunnelsBody lists every tunnel someone brought up, with its kind.
func (o *Overview) tunnelsBody(r *Renderer, csrf string) (template.HTML, error) {
	t := &Table{Columns: []TableColumn{
		{Label: "Tunnel", Kind: "mono"}, {Label: "Kind", Kind: "keyword"},
		{Label: "Receive", Kind: "rate"}, {Label: "Transmit", Kind: "rate"}, {Label: "Status", Kind: "status"},
	}, EmptyText: "No tunnel is up."}
	for _, n := range o.Interfaces {
		if n.tunnel() {
			t.Rows = append(t.Rows, interfaceRow(r.numbers(), n, TableCell{Text: n.Proto, Verbatim: true}))
		}
	}
	return o.renderWidget(r, t, csrf)
}

// ── Traffic ──

func (o *Overview) trafficHead(tr func(string) string) cardHead {
	nums := units.For(tr)
	down, up := nums.Rate(o.DownNow*1e6), nums.Rate(o.UpNow*1e6)
	return cardHead{Tone: "neutral", Summary: fmt.Sprintf(tr("%s down · %s up"), down, up)}
}

// trafficBody is the Internet graph, live over the last minute; the card's
// header names it, so the graph draws no title of its own.
func (o *Overview) trafficBody(r *Renderer, csrf string) (template.HTML, error) {
	down, up := o.DownSeries, o.UpSeries
	if len(down) < 2 || len(up) < 2 {
		down, up = make([]float64, wanSeriesFallback), make([]float64, wanSeriesFallback)
	}
	return renderToHTML(r, InternetTraffic(o.WANDevice, o.DownNow, o.UpNow, down, up, o.TrafficRangeURL, ""), csrf)
}

// overviewWindowView is the Traffic card's header control: the graph's window,
// where history stands behind it. The uplink's device is the Internet card's
// to name.
type overviewWindowView struct {
	RangeURL string
	Ranges   []TrafficRange
}

// trafficControl puts the graph's window in the card's header; choosing one
// fetches the graph for it into the card's body.
func (o *Overview) trafficControl(r *Renderer) (template.HTML, error) {
	if o.TrafficRangeURL == "" {
		return "", nil
	}
	v := overviewWindowView{RangeURL: o.TrafficRangeURL}
	for _, option := range TrafficRanges {
		v.Ranges = append(v.Ranges, TrafficRange{Value: option.Value, Label: r.tr(option.Label)})
	}
	return r.fragment("overview.window", v)
}

// ── Resources ──

// resourcesHead names the reading furthest out of band; in band it is silent.
// The gauges' labels arrive localized (they are the meters' own).
func (o *Overview) resourcesHead(tr func(string) string) cardHead {
	h := cardHead{Tone: "neutral"}
	parts := make([]string, 0, len(o.SysMetrics))
	rank := map[string]int{"warning": 1, "danger": 2}
	for _, m := range o.SysMetrics {
		parts = append(parts, strings.TrimSpace(m.Label+" "+m.Value+" "+m.Unit))
		if rank[m.Band] > rank[h.Tone] {
			h.Tone, h.Status = m.Band, fmt.Sprintf(tr("%s is high"), m.Label)
		}
	}
	h.Summary = joinSummary(parts...)
	return h
}

func (o *Overview) resourcesBody(r *Renderer, csrf string) (template.HTML, error) {
	meters := make([]template.HTML, 0, len(o.SysMetrics))
	for _, mt := range o.sysMeters() {
		html, err := o.renderWidget(r, mt, csrf)
		if err != nil {
			return "", err
		}
		meters = append(meters, html)
	}
	return r.fragment("overview.meters", meters)
}

// ── Hardware ──

// hardwareHead speaks only when the CPU runs hot; its line is the readings
// the board has sensors for.
func (o *Overview) hardwareHead(tr func(string) string) cardHead {
	temp, _, _ := strings.Cut(o.Temperature, " · ")
	read := func(v string) string {
		if v == "N/A" {
			return ""
		}
		return v
	}
	h := cardHead{Tone: "neutral", Summary: joinSummary(read(temp), read(o.Fan), read(o.Power))}
	if o.TempDot == "warning" || o.TempDot == "danger" {
		h.Tone, h.Status = o.TempDot, tr("CPU temperature is high")
	}
	return h
}

func (o *Overview) hardwareBody(r *Renderer, csrf string) (template.HTML, error) {
	sensors := o.sysRight()
	sensors.Items = append([]Property{sysProp("Board", o.Model, false)}, sensors.Items...)
	return o.renderWidget(r, sensors, csrf)
}

// ── Firmware ──

func (o *Overview) firmwareHead(tr func(string) string) cardHead {
	uptime := ""
	if o.Uptime != "" {
		uptime = fmt.Sprintf(tr("up %s"), o.Uptime)
	}
	return cardHead{Tone: "neutral", Summary: joinSummary(o.Firmware, uptime)}
}

func (o *Overview) firmwareBody(r *Renderer, csrf string) (template.HTML, error) {
	uptime := sysProp("Uptime", o.Uptime, false)
	uptime.Key = "uptime"
	return o.renderWidget(r, &Properties{Style: "overview-system", Items: []Property{
		sysProp("Firmware", o.Firmware, true),
		sysProp("Kernel", o.Kernel, true),
		uptime,
	}}, csrf)
}

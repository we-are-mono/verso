// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/units"
)

// cardHead is what a card's header says (ADR-022): its status word in a tone,
// and the one reading a closed card shows after its title, never the status
// again. A card of readings
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
}

// overviewCardSpecs are the Overview's cards, in the default layout's order.
func overviewCardSpecs() []overviewCardSpec {
	return []overviewCardSpec{
		{ID: "internet", Title: "Internet", Icon: "globe", Opens: "Interfaces", href: func(o *Overview) string { return o.InterfacesHref }, head: (*Overview).internetHead, body: (*Overview).internetBody},
		{ID: "ports", Title: "Ports", Icon: "ethernet-port", Opens: "Interfaces", href: func(o *Overview) string { return o.InterfacesHref }, head: (*Overview).portsHead, body: (*Overview).portsBody},
		{ID: "tunnels", Title: "Tunnels", Icon: "lock", Opens: "VPN", href: (*Overview).tunnelsHref, head: (*Overview).tunnelsHead, body: (*Overview).tunnelsBody},
		{ID: "traffic", Title: "Traffic", Icon: "activity", head: (*Overview).trafficHead, body: (*Overview).trafficBody},
		{ID: "resources", Title: "Resources", Icon: "gauge", head: (*Overview).resourcesHead, body: (*Overview).resourcesBody},
		{ID: "system", Title: "System", Icon: "router", head: (*Overview).systemHead, body: (*Overview).systemBody},
		{ID: "hardware", Title: "Hardware", Icon: "cpu", Opens: "Hardware", href: func(*Overview) string { return "/system/hardware" }, head: (*Overview).hardwareHead, body: (*Overview).hardwareBody},
	}
}

// ── Internet ──

// internetHead is the uplink's actual state and, while it is up, the address
// the internet sees it by.
func (o *Overview) internetHead(tr func(string) string) cardHead {
	if !o.WANKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	if !o.WANUp {
		return cardHead{Tone: "danger", Status: tr("Not connected")}
	}
	address, _, _ := strings.Cut(o.WANAddress, "/")
	return cardHead{Tone: "success", Status: tr("Connected"), Summary: address}
}

// overviewInternetView is the Internet card's body: the uplink's rows, then
// its address families, each a fact list headed by its name and protocol.
type overviewInternetView struct {
	Uplink   template.HTML
	Families []template.HTML
}

// internetBody is the uplink, its device and how long it has been connected
// (kept live by the stream), then the IPv4 and IPv6 facts, each address mono
// and copyable — the reading order for values a person compares line by line.
func (o *Overview) internetBody(r *Renderer, csrf string) (template.HTML, error) {
	families := []struct {
		kind, proto, empty string
		facts              []OverviewFact
	}{
		{"IPv4", o.V4Proto, "No address.", o.V4},
		{"IPv6", o.V6Proto, "No prefix was delegated.", o.V6},
	}
	view := overviewInternetView{Families: make([]template.HTML, 0, len(families))}
	if o.WANDevice != "" {
		uplink := []Property{{Label: "Device", Value: o.WANDevice, Mono: true}}
		if o.WANUp && o.WANUptime != nil {
			uplink = append(uplink, Property{Label: "Connected for", Duration: o.WANUptime, Key: "wan-uptime"})
		}
		rows, err := o.renderWidget(r, &Properties{Align: "left", Items: uplink}, csrf)
		if err != nil {
			return "", err
		}
		view.Uplink = rows
	}
	for _, f := range families {
		list := &Properties{Align: "left", Head: &PropertiesHead{Label: f.kind, Chip: overviewProtocol(f.proto)}, Empty: f.empty}
		if len(f.facts) > 0 && (len(f.facts) > 1 || f.facts[0].Value != "Not configured") {
			for _, fact := range f.facts {
				list.Items = append(list.Items, Property{Label: fact.Label, Value: fact.Value, Mono: fact.Copy, Copy: fact.Copy, Duration: fact.Duration})
			}
		}
		rows, err := o.renderWidget(r, list, csrf)
		if err != nil {
			return "", err
		}
		view.Families = append(view.Families, rows)
	}
	return r.fragment("overview.internet", view)
}

// ── Ports ──

// portsHead speaks when a port something asked for has lost its cable, and
// says how many devices hang off the ports. Every such port linked is what the
// panel and the list under the header show, so then it says nothing. A spare
// port's missing cable is nobody's concern.
func (o *Overview) portsHead(tr func(string) string) cardHead {
	if !o.InterfacesKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	ports, down, unknown := 0, 0, 0
	for _, n := range o.Interfaces {
		if !n.port() || !n.inUse() {
			continue
		}
		ports++
		switch n.State {
		case "up":
		case "down", "lowerlayerdown":
			down++
		default:
			unknown++
		}
	}
	h := cardHead{Tone: "neutral"}
	if o.DevicesKnown {
		h.Summary = deviceCount(tr, o.DevicesOnline)
	}
	switch {
	case down > 0:
		h.Tone = "warning"
		h.Status = overviewCount(tr, down, "%d port without link", "%d ports without link (two)", "%d ports without link (few)", "%d ports without link")
	case unknown > 0:
		h.Status = tr("State unknown")
	case ports > 0:
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

// portsBody is the router's panel over the list of its ports: the network each
// carries, what moves on it now, and its link, status last. The list follows
// the panel's left-to-right order, so the card has one order.
func (o *Overview) portsBody(r *Renderer, csrf string) (template.HTML, error) {
	t := &Table{Columns: []TableColumn{
		{Label: "Port", Kind: "mono"}, {Label: "Network", Kind: "entity"},
		{Label: "Receive", Kind: "rate"}, {Label: "Transmit", Kind: "rate"}, {Label: "Status", Kind: "status"},
	}, EmptyText: "No physical ports were found."}
	for _, n := range o.portsInPanelOrder() {
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
	table, err := o.renderWidget(r, t, csrf)
	if err != nil || o.Panel == nil {
		return table, err
	}
	panel, err := o.renderWidget(r, o.Panel, csrf)
	if err != nil {
		return "", err
	}
	return r.fragment("overview.ports", overviewPortsView{Panel: panel, Table: table})
}

type overviewPortsView struct {
	Panel, Table template.HTML
}

// portsInPanelOrder is every port on the case, in the panel's left-to-right
// order; a port the panel does not draw follows in the kernel's order.
func (o *Overview) portsInPanelOrder() []OverviewInterface {
	place := map[string]int{}
	if o.Panel != nil {
		for i, it := range o.Panel.Items {
			place[it.Iface] = i + 1
		}
	}
	var out []OverviewInterface
	for _, n := range o.Interfaces {
		if n.port() {
			out = append(out, n)
		}
	}
	slices.SortStableFunc(out, func(a, b OverviewInterface) int {
		pa, pb := place[a.Name], place[b.Name]
		switch {
		case pa == pb:
			return 0
		case pa == 0:
			return 1
		case pb == 0:
			return -1
		default:
			return pa - pb
		}
	})
	return out
}

// ── Tunnels ──

func (o *Overview) tunnelsHref() string {
	if o.TunnelsHref != "" {
		return o.TunnelsHref
	}
	return o.InterfacesHref
}

// tunnelsHead reads each tunnel's netdev, which says nothing about a remote
// peer or a handshake: up is the device, not the far end. Its line names the
// tunnels that are down, else counts them.
func (o *Overview) tunnelsHead(tr func(string) string) cardHead {
	if !o.InterfacesKnown {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	var downNames []string
	tunnels, up := 0, 0
	for _, n := range o.Interfaces {
		if !n.tunnel() {
			continue
		}
		tunnels++
		switch n.State {
		case "up":
			up++
		case "down", "lowerlayerdown":
			downNames = append(downNames, n.Name)
		}
	}
	h := cardHead{Tone: "neutral"}
	switch {
	case tunnels == 0:
		h.Status = tr("None observed")
	case len(downNames) > 0:
		h.Tone, h.Status = "danger", fmt.Sprintf(tr("%d of %d down"), len(downNames), tunnels)
		h.Summary = strings.Join(downNames, ", ")
	case up == tunnels:
		h.Tone, h.Status = "success", tr("All up")
	default:
		h.Status = tr("State unknown")
	}
	if h.Summary == "" && tunnels > 0 {
		h.Summary = tunnelCount(tr, tunnels)
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
	return renderToHTML(r, InternetTraffic(o.DownNow, o.UpNow, down, up, o.TrafficRangeURL, ""), csrf)
}

// ── Resources ──

// resourcesHead names the reading furthest out of band; in band it is silent.
// Its line is always memory, what a router runs out of, so the line never
// swaps one reading for another as they pass each other. The gauges' labels
// arrive localized (they are the meters' own).
func (o *Overview) resourcesHead(tr func(string) string) cardHead {
	h := cardHead{Tone: "neutral"}
	rank := map[string]int{"warning": 1, "danger": 2}
	for _, m := range o.SysMetrics {
		if m.Name == "sys-memory" {
			h.Summary = strings.TrimSpace(m.Label + " " + m.Value + " " + m.Unit)
		}
		if rank[m.Band] > rank[h.Tone] {
			h.Tone, h.Status = m.Band, fmt.Sprintf(tr("%s is high"), m.Label)
		}
	}
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

// ── System ──

// systemHead's line is the firmware the router runs; the board, what it was
// built for and how long it has been up are in the open card.
func (o *Overview) systemHead(func(string) string) cardHead {
	return cardHead{Tone: "neutral", Summary: o.Firmware}
}

func (o *Overview) systemBody(r *Renderer, csrf string) (template.HTML, error) {
	uptime := Property{Label: "Uptime", Value: "unavailable", Key: "uptime"}
	if o.Uptime != nil {
		uptime = Property{Label: "Uptime", Duration: o.Uptime, Key: "uptime"}
	}
	// The router's clock, in its zone, kept live by the stream.
	clock := Property{Label: "Local time", Value: "unavailable", Key: "local-time"}
	if o.Clock != "" {
		clock = Property{Label: "Local time", Value: localTime(o.Clock, o.Zone), Mono: true, Verbatim: true, Key: "local-time"}
	}
	return o.renderWidget(r, &Properties{Items: []Property{
		sysProp("Board", o.Model, false),
		sysProp("Architecture", o.Arch, true),
		sysProp("Target platform", o.Target, true),
		sysProp("Firmware", o.Firmware, true),
		sysProp("Kernel", o.Kernel, true),
		uptime,
		clock,
	}}, csrf)
}

// ── Hardware ──

// OverviewSensors is the Hardware card: the board's headline readings, as the
// profile names them and the server writes them; nil where none answered.
// The Hardware page holds the rest.
type OverviewSensors struct {
	CPU, Power, Fan *OverviewSensor
}

// OverviewSensor is one reading: Key tags its row for the stream, and Tone is
// "warning" or "danger" for a CPU past its own limit or a stopped fan.
type OverviewSensor struct {
	Key, Name, Value, Tone string
}

func (s *OverviewSensors) rows() []*OverviewSensor {
	if s == nil {
		return nil
	}
	return slices.DeleteFunc([]*OverviewSensor{s.CPU, s.Power, s.Fan}, func(r *OverviewSensor) bool { return r == nil })
}

// Verdict grades the hardware for whatever speaks for it (the Overview
// card's header, the Hardware page's heading): a hot CPU in its own tone,
// else a stopped fan, in words; none, the empty tone and words, while both
// are in band.
func (s *OverviewSensors) Verdict(tr func(string) string) (tone, words string) {
	switch {
	case s == nil:
	case s.CPU != nil && s.CPU.Tone != "":
		return s.CPU.Tone, fmt.Sprintf(tr("%s is hot"), tr(s.CPU.Name))
	case s.Fan != nil && s.Fan.Tone != "":
		return s.Fan.Tone, fmt.Sprintf(tr("%s stopped"), tr(s.Fan.Name))
	}
	return "", ""
}

// hardwareHead speaks the hardware's verdict, and its line is always the CPU
// temperature.
func (o *Overview) hardwareHead(tr func(string) string) cardHead {
	hw := o.Hardware
	if len(hw.rows()) == 0 {
		return cardHead{Tone: "neutral", Status: tr("Unavailable")}
	}
	h := cardHead{Tone: "neutral"}
	if tone, words := hw.Verdict(tr); tone != "" {
		h.Tone, h.Status = tone, words
	}
	if hw.CPU != nil {
		h.Summary = hw.CPU.Value
	}
	return h
}

// hardwareBody is one fact list, its values on the right edge so a long name
// keeps its line.
func (o *Overview) hardwareBody(r *Renderer, csrf string) (template.HTML, error) {
	list := &Properties{Empty: "No sensor answered."}
	for _, s := range o.Hardware.rows() {
		list.Items = append(list.Items, Property{Label: s.Name, Value: s.Value, Verbatim: true, Key: s.Key})
	}
	return o.renderWidget(r, list, csrf)
}

// LocalTime is the router's clock as the System card reads it, its zone
// after it ("14:52:07 CEST"); the page's first paint and the stream write it
// alike.
func LocalTime(t time.Time) string {
	zone, _ := t.Zone()
	return localTime(t.Format("15:04:05"), zone)
}

func localTime(clock, zone string) string { return strings.TrimSpace(clock + " " + zone) }

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Overview composes the shell's landing page from observed router state: a
// verdict over cards that each hold one kind of reading (ADR-022). It is
// outside the plugin vocabulary; the cards' bodies reuse the existing
// renderers.
type Overview struct {
	Clock, Zone     string
	Unix            int64
	Offset          int
	InterfacesKnown bool
	TunnelsHref     string
	InterfacesHref  string

	// Layout is where the cards stand, read from the browser (ADR-022); the
	// zero value is the default layout.
	Layout OverviewLayout

	Firmware  string
	Kernel    string
	Uptime    string
	WANKnown  bool
	WANUp     bool
	WANDevice string
	WANUptime string

	DownSeries []float64
	UpSeries   []float64
	DownVal    string
	UpVal      string
	// TrafficRangeURL offers the Internet graph's window where history
	// stands behind it (Traffic.RangeURL).
	TrafficRangeURL string

	// The IPv4/IPv6 connection facts. Each side carries its protocol label and
	// its rows; an absent address family says so in its place.
	V4Proto string
	V4      []OverviewFact
	V6Proto string
	V6      []OverviewFact

	// SysMetrics are the live System gauges (load, CPU, memory, storage); empty
	// renders an unavailable state.
	SysMetrics []OverviewMeter

	// Model is the board's human name. Temperature/Fan/Power are the resolved
	// hardware sensor facts (see internal/sensors); an absent reading shows N/A.
	// TempDot is the temperature status tone ("success"|"warning"|"danger").
	Model       string
	Temperature string
	TempDot     string
	Fan         string
	Power       string

	// Interfaces is the live listing: every kernel interface, enriched with its
	// topology/UCI meaning.
	Interfaces []OverviewInterface

	// The Ports card's line includes the observed device count when available.
	DevicesOnline int
	DevicesKnown  bool

	// SecurityHref is resolved from the live plugin registry, not a guessed URL.
	SecurityHref string
	// FirewallState comes from fw4's loaded kernel table, not its configuration.
	// Empty means the runtime read failed. The count uses the same UCI rule
	// sections as the Firewall page, including disabled and staged rules.
	FirewallState      string
	FirewallRules      int
	FirewallRulesKnown bool
}

// OverviewInterface is one kernel interface enriched with runtime topology and
// any matching UCI network meaning.
type OverviewInterface struct {
	Name      string
	Kind      string
	State     string
	Physical  bool
	WAN       bool
	Networks  []string
	Relations []OverviewInterfaceRelation
	VLAN      string
	Subnet    string
	Zone      string
	Proto     string
	RxRate    string
	TxRate    string
	RxTotal   string
	TxTotal   string
	RxPackets string
	TxPackets string
}

// inUse is whether anything asked for the port: a network sits on it, or it is
// a bridge's member or a VLAN's parent. A spare port is neither, so its missing
// cable is nobody's concern.
func (n OverviewInterface) inUse() bool {
	return len(n.Networks) > 0 || len(n.Relations) > 0
}

// kernelTunnels are the devices a tunnel module makes for itself when it
// loads, under these fixed names, whether or not anyone configured a tunnel.
// They are nobody's tunnels; every other tunnel device someone brought up.
var kernelTunnels = map[string]bool{
	"sit0": true, "ip6tnl0": true, "tunl0": true, "gre0": true, "gretap0": true,
	"erspan0": true, "ip6gre0": true, "ip_vti0": true, "ip6_vti0": true,
}

// tunnel is whether the interface is a tunnel someone brought up.
func (n OverviewInterface) tunnel() bool {
	return n.Kind == "tunnel" && !kernelTunnels[n.Name]
}

// OverviewInterfaceRelation is a parent or member shown in the topology cell.
type OverviewInterfaceRelation struct {
	Name     string
	Physical bool
}

// OverviewFact is one connection-facts row: a label, its value, and whether the
// value offers an inline copy control (the addresses do).
type OverviewFact struct {
	Label string
	Value string
	Copy  bool
}

// OverviewMeter is one live System gauge — a named bar-layout meter with its
// health band, big value, fill, and caption. The Name lets the overview
// stream update it in place.
type OverviewMeter struct {
	Name   string
	Label  string
	Icon   string
	Role   string
	Band   string
	Value  string
	Unit   string
	Detail string
	Fill   int
}

// InternetTraffic is the overview's Internet graph of the uplink device, in
// the Traffic card: the live minute when window is empty or "minute", else the
// past window drawn from history, its figures the window's averages.
func InternetTraffic(device, downNow, upNow string, down, up []float64, rangeURL, window string) *Traffic {
	label := "Internet traffic — download and upload, last minute"
	if window != "" && window != TrafficRanges[0].Value {
		label = "Internet traffic — download and upload"
	}
	return &Traffic{
		Title: "Internet traffic", Meta: device, Live: "wan", Label: label,
		DownNow: downNow, UpNow: upNow, Down: down, Up: up,
		RangeURL: rangeURL, Range: window, Card: true,
	}
}

// wanSeriesFallback is the flat-line length drawn before any WAN sample lands.
const wanSeriesFallback = 60

func (*Overview) isWidget() {}

// children is empty: the overview composes its cards at render time, so its
// renderInto runs the localization walk over each widget itself.
func (*Overview) children() []Widget { return nil }

// overviewVerdictView is the page's one sentence and its live clock.
type overviewVerdictView struct {
	Tone, Lead, Accent, Tail string
	Clock, Zone              string
	Unix                     int64
	Offset                   int
}

// overviewCardView is one card as the page draws it.
type overviewCardView struct {
	ID, Title, Icon string
	Head            cardHead
	Closed          bool
	Href, OpenLabel string
	Body            template.HTML
}

// overviewShelfView is a hidden card on the Arrange shelf, wearing its tone.
type overviewShelfView struct {
	ID, Title, Tone string
}

// overviewView is the page's layout model: the verdict, the two columns of
// cards in the browser's order, and the hidden cards.
type overviewView struct {
	Verdict overviewVerdictView
	Cols    [2][]overviewCardView
	Hidden  []overviewShelfView
}

// layout is the browser's layout, or the default where none was read.
func (o *Overview) layout() OverviewLayout {
	if len(o.Layout.Cols[0])+len(o.Layout.Cols[1]) == 0 {
		return DefaultOverviewLayout()
	}
	return o.Layout
}

func (o *Overview) renderInto(r *Renderer, out io.Writer, csrf string) error {
	layout := o.layout()
	specs := map[string]overviewCardSpec{}
	for _, spec := range overviewCardSpecs() {
		specs[spec.ID] = spec
	}
	v := overviewView{Verdict: o.verdict(r.tr)}
	for col, ids := range layout.Cols {
		for _, id := range ids {
			card, err := o.card(r, specs[id], layout.IsClosed(id), csrf)
			if err != nil {
				return err
			}
			v.Cols[col] = append(v.Cols[col], card)
		}
	}
	for _, id := range layout.Hidden {
		spec := specs[id]
		v.Hidden = append(v.Hidden, overviewShelfView{ID: id, Title: r.tr(spec.Title), Tone: spec.head(o, r.tr).Tone})
	}
	return r.execute(out, "overview.html.tmpl", v)
}

// card builds one shown card: its header from its head, its body from its
// builder, and its door from the live registry.
func (o *Overview) card(r *Renderer, spec overviewCardSpec, closed bool, csrf string) (overviewCardView, error) {
	body, err := spec.body(o, r, csrf)
	if err != nil {
		return overviewCardView{}, fmt.Errorf("overview card %s: %w", spec.ID, err)
	}
	v := overviewCardView{ID: spec.ID, Title: r.tr(spec.Title), Icon: spec.Icon, Head: spec.head(o, r.tr), Closed: closed, Body: body}
	if spec.href != nil {
		if v.Href = spec.href(o); v.Href != "" {
			v.OpenLabel = fmt.Sprintf(r.tr("Open %s"), r.tr(spec.Opens))
		}
	}
	return v, nil
}

// renderWidget localizes one composed widget and renders it to a fragment —
// the overview's per-widget path through the same walk a plugin tree takes.
func (o *Overview) renderWidget(r *Renderer, w Widget, csrf string) (template.HTML, error) {
	r.translate(w)
	return renderToHTML(r, w, csrf)
}

// sysProp builds one System fact row. A live value is data (a model name, an
// uptime) and declares itself Verbatim; a missing one degrades to
// "unavailable" — prose, left in source form for the schema walk to localize
// exactly once, without the mono treatment.
func sysProp(label, value string, mono bool) Property {
	if value == "" {
		return Property{Label: label, Value: "unavailable"}
	}
	return Property{Label: label, Value: value, Mono: mono, Verbatim: true}
}

// sysMeters builds the System gauges from the live fields, each a named bar meter
// so the stream can update it in place.
func (o *Overview) sysMeters() []*Meter {
	out := make([]*Meter, 0, len(o.SysMetrics))
	for _, m := range o.SysMetrics {
		out = append(out, &Meter{
			Name: m.Name, Icon: m.Icon, Overview: true, Tone: m.Band, Verbatim: true,
			Label: m.Label, Value: m.Value, Unit: m.Unit, Fill: m.Fill, Detail: m.Detail,
		})
	}
	return out
}

// sysRight keeps three sensor rows, with N/A for an unavailable reading. Keys
// tag each value for the overview stream, including a sensor that appears later.
func (o *Overview) sysRight() *Properties {
	reading := func(value string) string {
		if value == "" {
			return "N/A"
		}
		return value
	}
	value, note, _ := strings.Cut(reading(o.Temperature), " · ")
	tone := o.TempDot
	if tone == "" {
		tone = "neutral"
	}
	return &Properties{Style: "overview-system", Items: []Property{
		{Label: "CPU Temperature", Value: value, Help: note, HelpVerbatim: true, Mono: true, Verbatim: true, Dot: tone, Key: "temperature"},
		{Label: "Fan speed", Value: reading(o.Fan), Mono: true, Verbatim: true, Key: "fan"},
		{Label: "Power draw", Value: reading(o.Power), Mono: true, Verbatim: true, Key: "power"},
	}}
}

// renderToHTML renders one widget to a fragment for injection into a composing
// template (the overview places the chart and the listings itself).
func renderToHTML(r *Renderer, w Widget, csrf string) (template.HTML, error) {
	var b strings.Builder
	if err := r.render(&b, w, csrf); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// fragment renders one of the page's own named templates to a fragment.
func (r *Renderer) fragment(name string, data any) (template.HTML, error) {
	var b strings.Builder
	if err := r.execute(&b, name, data); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func overviewProtocol(proto string) string {
	switch proto {
	case "DHCPv6 client":
		return "dhcpv6"
	case "DHCP":
		return "dhcp"
	case "PPPoE":
		return "pppoe"
	case "Static":
		return "static"
	}
	return proto
}

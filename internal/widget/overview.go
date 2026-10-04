// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Overview composes the shell's landing page from observed router state. It is
// outside the plugin vocabulary; charts, meters and properties reuse their
// existing renderers.
type Overview struct {
	Clock, Zone     string
	Unix            int64
	Offset          int
	InterfacesKnown bool
	TunnelsHref     string
	InterfacesHref  string

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

	// The IPv4/IPv6 connection facts. Each side carries its protocol label and
	// its rows; an absent address family keeps its place when facts are available.
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

	// The Interfaces tile includes the observed device count when available.

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

// wanSeriesFallback is the flat-line length drawn before any WAN sample lands.
const wanSeriesFallback = 60

func (*Overview) isWidget() {}

// children is empty: the overview composes its chart, meters, and tables at
// render time, so its renderInto runs the localization walk over each itself.
func (*Overview) children() []Widget { return nil }

// ohTile is one status tile in the strip under the verdict: an eyebrow label,
// a trailing icon, a status dot + word, and a caption. Page-owned chrome (like
// the traffic legend), not a widget: its anatomy — the dot beside the word,
// the quiet caption — is this page's own. Variant speaks the tone vocabulary.
// A tile with an Href is also a doorway: the whole tile becomes the link.
type ohTile struct {
	ID       string
	Identity string
	Label    string
	Icon     string
	Variant  string // "neutral" | "success" | "warning" | "danger"
	Status   string
	Caption  string
	Key      string // live hook: the stream refreshes the caption in place
	Href     string
}

// ohFactColView is one connection-facts column (IPv4 or IPv6): an eyebrow
// (kind + protocol) over its rows — a Properties widget rendered to HTML. Side
// places it left or right of the divider.
type ohFactColView struct {
	Empty string
	Kind  string
	Proto string
	Side  string // "left" | "right"
	Rows  template.HTML
}

// overviewMastheadView is the top block's render model — the verdict, the view
// switch, the status tiles, and the connection facts.
type overviewMastheadView struct {
	Tone                string
	Clock, Zone, Uptime string
	Unix                int64
	Offset              int

	Kicker string
	Lead   string
	Accent string
	Tail   string
	Tiles  []ohTile
	Facts  []ohFactColView
}

// overviewView is the page's layout model: the vocabulary widgets — the fact
// sheets, the gauges, the listings — rendered to HTML, plus the page's own
// chrome (the masthead block, the traffic panel's legend). The page owns
// composition and rhythm.
type overviewView struct {
	Masthead overviewMastheadView

	ChartTitle  string
	ChartMeta   string
	DownVal     string
	UpVal       string
	RateUnit    string
	Chart       template.HTML
	TrafficSeed string // {"down":[…],"up":[…]} — the live layer's starting series

	Metrics  []template.HTML
	SysLeft  template.HTML
	SysRight template.HTML
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

func (o *Overview) renderInto(r *Renderer, out io.Writer, csrf string) error {
	// The plot draws the real WAN series; before any sample is observed it falls
	// back to a flat quiet line so the axis and shape hold.
	down, up := o.DownSeries, o.UpSeries
	if len(down) < 2 || len(up) < 2 {
		down, up = make([]float64, wanSeriesFallback), make([]float64, wanSeriesFallback)
	}
	chart := &Chart{
		Unit:      "Mbit/s",
		AxisStart: "60 s ago",
		AxisEnd:   "now",
		Label:     "Internet traffic — download and upload, last minute",
		Series: []ChartSeries{
			{Label: "down", Role: "emerald", Fill: true, Values: down},
			{Label: "up", Role: "violet", Fill: true, Values: up},
		},
	}
	// The overview composes its child widgets directly (not through
	// RenderWithToken), so it runs the localization walk over each itself — every
	// column label, cell word, chart caption and drawer fact is translated in one
	// place, exactly as a plugin's table would be (ADR-012).
	r.translate(chart)
	chartHTML, err := renderToHTML(r, chart, csrf)
	if err != nil {
		return err
	}
	seed, err := json.Marshal(map[string][]float64{"down": down, "up": up})
	if err != nil {
		return err
	}

	// The System meters share the server-computed health bands with the stream.
	metrics := make([]template.HTML, 0, 4)
	for _, mt := range o.sysMeters() {
		r.translate(mt)
		html, err := renderToHTML(r, mt, csrf)
		if err != nil {
			return err
		}
		metrics = append(metrics, html)
	}

	tiles := []ohTile{o.internetTile(r.tr), o.firewallTile(r.tr), o.tunnelTile(r.tr), o.interfacesTile(r.tr)}

	facts, err := o.factCols(r, csrf)
	if err != nil {
		return err
	}
	sysLeft, err := o.renderWidget(r, &Properties{Style: "overview-system", Items: []Property{
		sysProp("Device", o.Model, false),
		sysProp("Firmware", o.Firmware, true),
		sysProp("Kernel", o.Kernel, true),
	}}, csrf)
	if err != nil {
		return err
	}
	sysRight, err := o.renderWidget(r, o.sysRight(), csrf)
	if err != nil {
		return err
	}

	masthead := o.masthead(r.tr)
	masthead.Tiles, masthead.Facts = tiles, facts
	v := overviewView{
		Masthead:    masthead,
		ChartTitle:  r.tr("Internet traffic"),
		ChartMeta:   o.WANDevice,
		DownVal:     o.DownVal,
		UpVal:       o.UpVal,
		RateUnit:    "Mbit/s",
		Chart:       chartHTML,
		TrafficSeed: string(seed),

		Metrics:  metrics,
		SysLeft:  sysLeft,
		SysRight: sysRight,
	}
	return r.execute(out, "overview.html.tmpl", v)
}

// renderWidget localizes one composed widget and renders it to a fragment —
// the overview's per-widget path through the same walk a plugin tree takes.
func (o *Overview) renderWidget(r *Renderer, w Widget, csrf string) (template.HTML, error) {
	r.translate(w)
	return renderToHTML(r, w, csrf)
}

// internetTile is the one live tile: the WAN's actual state, its caption
// refreshed in place by the stream (Key).
func (o *Overview) internetTile(tr func(string) string) ohTile {
	tile := ohTile{ID: "internet", Label: tr("Internet"), Icon: "globe", Variant: "neutral", Status: tr("Unavailable"), Key: "internet-uptime", Identity: o.WANDevice}
	if !o.WANKnown {
		return tile
	}
	if !o.WANUp {
		tile.Variant, tile.Status, tile.Caption = "danger", tr("Not connected"), tr("WAN is down")
		return tile
	}
	tile.Variant, tile.Status = "success", tr("Connected")
	if o.WANUptime != "" {
		tile.Caption = fmt.Sprintf(tr("for %s"), o.WANUptime)
	}
	return tile
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

// factCols builds the IPv4/IPv6 connection-facts columns from the live fields:
// each side an eyebrow over a left-aligned Properties sheet (mono, copyable —
// the reading order for addresses a person compares line by line). A page that
// carries neither side draws no columns at all.
func (o *Overview) factCols(r *Renderer, csrf string) ([]ohFactColView, error) {
	if len(o.V4) == 0 && len(o.V6) == 0 {
		return nil, nil
	}
	sheet := func(facts []OverviewFact) *Properties {
		items := make([]Property, 0, len(facts))
		for _, f := range facts {
			items = append(items, Property{Label: f.Label, Value: f.Value, Mono: f.Copy, Emphasis: true, Copy: f.Copy, Verbatim: f.Label == "Expires"})
		}
		return &Properties{Style: "overview-connection", Align: "left", Items: items}
	}
	v4, err := o.renderWidget(r, sheet(o.V4), csrf)
	if err != nil {
		return nil, err
	}
	v6, err := o.renderWidget(r, sheet(o.V6), csrf)
	if err != nil {
		return nil, err
	}
	cols := []ohFactColView{
		{Kind: "IPv4", Proto: overviewProtocol(o.V4Proto), Side: "left", Rows: v4},
		{Kind: "IPv6", Proto: overviewProtocol(o.V6Proto), Side: "right", Rows: v6},
	}
	if len(o.V6) == 1 && o.V6[0].Value == "Not configured" {
		cols[1].Empty = r.tr("No IPv6 connection · no prefix was delegated")
	}
	return cols, nil
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

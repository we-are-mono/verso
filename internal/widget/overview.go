// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
	"strings"
)

// Overview is the advanced overview page, transferred hardcoded from the design:
// a verdict line with a Basic/Advanced toggle, a strip of four status tiles, the
// IPv4/IPv6 connection facts, the internet-traffic graph, the System panel, and
// the Interfaces / Connected devices / DHCP leases listings. It is the shell's own
// page content — not part of the plugin-facing vocabulary, so it never appears in
// Decode — a faithful transfer that later steps wire to live data.
//
// The graph reuses the generic Chart widget and the listings reuse the generic
// flat Table widget (both injected as rendered HTML); Overview owns only the page
// composition — the verdict, the tiles, the facts, and the section rhythm. The
// verdict headline is set in the serif display face (Fraunces).
//
// Firmware, Kernel, and Uptime are the live System facts, and DownSeries/UpSeries
// (with DownVal/UpVal, the newest values) the live WAN traffic minute — all filled
// by the shell from the backend before rendering; the rest is placeholder pending
// its own wiring.
type Overview struct {
	Firmware string
	Kernel   string
	Uptime   string

	DownSeries []float64
	UpSeries   []float64
	DownVal    string
	UpVal      string

	// The IPv4/IPv6 connection facts. Each side carries its protocol label and
	// its rows; empty falls back to a placeholder so the panel always renders.
	V4Proto string
	V4      []OverviewFact
	V6Proto string
	V6      []OverviewFact

	// SysMetrics are the live System gauges (load, CPU, memory, storage); empty
	// falls back to a static placeholder.
	SysMetrics []OverviewMeter

	// Model is the board's human name. Temperature/Fan/Power/SensorSummary are
	// the resolved hardware sensor facts (see internal/sensors); an empty string
	// hides that row — an unprofiled PC shows no power draw rather than a zero.
	// TempDot is the temperature status colour ("emerald"|"amber"|"red").
	Model         string
	Temperature   string
	TempDot       string
	Fan           string
	Power         string
	SensorSummary string
}

// OverviewFact is one connection-facts row: a label, its value, and whether the
// value offers an inline copy control (the addresses do).
type OverviewFact struct {
	Label string
	Value string
	Copy  bool
}

// OverviewMeter is one live System gauge — a named bar-layout meter with its
// decorative accent, big value, fill, and caption. The Name lets the overview
// stream update it in place.
type OverviewMeter struct {
	Name   string
	Label  string
	Icon   string
	Role   string
	Value  string
	Unit   string
	Detail string
	Fill   int
}

// wanSeriesFallback is the flat-line length drawn before any WAN sample lands.
const wanSeriesFallback = 60

func (*Overview) isWidget() {}

// ohTile is one status tile: an eyebrow label, a trailing icon, a status dot +
// word, and a caption. Variant "good" tints emerald; "warning" tints amber.
type ohTile struct {
	Label   string
	Icon    string
	Variant string // "good" | "warning"
	Status  string
	Caption string
}

// ohRow is one label/value fact; the value is machine text set in mono, with an
// inline copy control when Copy is set.
type ohRow struct {
	Label string
	Value string
	Copy  bool
}

// ohFactCol is one connection-facts column (IPv4 or IPv6): an eyebrow (kind +
// protocol) over its rows. Side places it left or right of the divider.
type ohFactCol struct {
	Kind  string
	Proto string
	Side  string // "left" | "right"
	Rows  []ohRow
}

// ohProp is one System fact row: a label and its value, mono for machine strings,
// with an optional leading status dot (Dot names the colour, e.g. "emerald"). Key
// tags a live-updated sensor row (temperature/fan/power/summary) so the overview
// stream can refresh its value — and the dot's colour — in place.
type ohProp struct {
	Label string
	Value string
	Mono  bool
	Dot   string
	Key   string
}

type overviewView struct {
	Kicker string
	Lead   string
	Accent string
	Tiles  []ohTile
	Facts  []ohFactCol

	ChartTitle  string
	ChartMeta   string
	DownVal     string
	UpVal       string
	RateUnit    string
	Chart       template.HTML
	TrafficSeed string // {"down":[…],"up":[…]} — the live layer's starting series

	SysMeta  string
	Metrics  []template.HTML
	SysLeft  []ohProp
	SysRight []ohProp

	Interfaces template.HTML
	DhcpLeases template.HTML
}

// orUnavailable falls a missing live fact back to a plain "unavailable" so the
// System panel degrades honestly rather than showing a stale placeholder.
func orUnavailable(s string) string {
	if s == "" {
		return "unavailable"
	}
	return s
}

func (o *Overview) renderInto(r *Renderer, out io.Writer, csrf string) error {
	// The plot draws the real WAN series; before any sample is observed it falls
	// back to a flat quiet line so the axis and shape hold.
	down, up := o.DownSeries, o.UpSeries
	if len(down) < 2 || len(up) < 2 {
		down, up = make([]float64, wanSeriesFallback), make([]float64, wanSeriesFallback)
	}
	chart := &Chart{
		Size:      "panel",
		Axis:      true,
		Max:       300,
		AxisStart: "60 seconds ago",
		AxisEnd:   "now",
		Label:     "Internet traffic — download and upload, last minute",
		Series: []ChartSeries{
			{Label: "down", Role: "sky", Fill: true, Values: down},
			{Label: "up", Role: "violet", Fill: true, Values: up},
		},
	}
	chartHTML, err := renderToHTML(r, chart, csrf)
	if err != nil {
		return err
	}
	seed, err := json.Marshal(map[string][]float64{"down": down, "up": up})
	if err != nil {
		return err
	}
	interfaces, err := renderToHTML(r, overviewInterfaces(), csrf)
	if err != nil {
		return err
	}
	leases, err := renderToHTML(r, overviewLeases(), csrf)
	if err != nil {
		return err
	}

	// The System headline metrics as bar-layout Meters — each a fixed accent
	// (icon + bar), not a health band, so the row reads as a dashboard.
	metrics := make([]template.HTML, 0, 4)
	for _, mt := range o.sysMeters() {
		html, err := renderToHTML(r, mt, csrf)
		if err != nil {
			return err
		}
		metrics = append(metrics, html)
	}

	v := overviewView{
		Kicker: "ALL GOOD",
		Lead:   "Your network is ",
		Accent: "healthy",
		Tiles: []ohTile{
			{Label: "INTERNET", Icon: "globe", Variant: "good", Status: "Connected", Caption: "for 2h 14m"},
			{Label: "WI-FI", Icon: "wifi", Variant: "good", Status: "Both bands active", Caption: "2.4 & 5 GHz"},
			{Label: "SECURITY", Icon: "shield", Variant: "good", Status: "Protected", Caption: "Firewall on"},
			{Label: "SOFTWARE", Icon: "download", Variant: "warning", Status: "Update available", Caption: "Security fixes"},
		},
		Facts:       o.factCols(),
		ChartTitle:  "Internet traffic",
		ChartMeta:   "live · WAN",
		DownVal:     o.DownVal,
		UpVal:       o.UpVal,
		RateUnit:    "Mbps",
		Chart:       chartHTML,
		TrafficSeed: string(seed),

		SysMeta: "hardware · live",
		Metrics: metrics,
		SysLeft: []ohProp{
			{Label: "Model", Value: orUnavailable(o.Model)},
			{Label: "Firmware", Value: orUnavailable(o.Firmware), Mono: true},
			{Label: "Kernel", Value: orUnavailable(o.Kernel), Mono: true},
			{Label: "Uptime", Value: orUnavailable(o.Uptime)},
		},
		SysRight: o.sysRight(),

		Interfaces: interfaces,
		DhcpLeases: leases,
	}
	return r.execute(out, "overview.html.tmpl", v)
}

// sysMeters builds the System gauges from the live fields, each a named bar meter
// so the stream can update it in place.
func (o *Overview) sysMeters() []*Meter {
	out := make([]*Meter, 0, len(o.SysMetrics))
	for _, m := range o.SysMetrics {
		out = append(out, &Meter{
			Layout: "bar", Name: m.Name, Icon: m.Icon, Role: m.Role,
			Label: m.Label, Value: m.Value, Unit: m.Unit, Fill: m.Fill, Detail: m.Detail,
		})
	}
	return out
}

// sysRight builds the hardware-sensor facts column, omitting any reading the box
// doesn't expose — a PC with no power sensor simply shows no "Power draw" row,
// never a fabricated zero.
func (o *Overview) sysRight() []ohProp {
	rows := make([]ohProp, 0, 4)
	if o.Temperature != "" {
		rows = append(rows, ohProp{Label: "Temperature", Value: o.Temperature, Dot: o.TempDot, Key: "temperature"})
	}
	if o.Fan != "" {
		rows = append(rows, ohProp{Label: "Fan", Value: o.Fan, Key: "fan"})
	}
	if o.Power != "" {
		rows = append(rows, ohProp{Label: "Power draw", Value: o.Power, Key: "power"})
	}
	if o.SensorSummary != "" {
		rows = append(rows, ohProp{Label: "Sensors", Value: o.SensorSummary, Key: "summary"})
	}
	return rows
}

// factCols builds the IPv4/IPv6 connection-facts columns from the live fields.
func (o *Overview) factCols() []ohFactCol {
	rows := func(facts []OverviewFact) []ohRow {
		out := make([]ohRow, 0, len(facts))
		for _, f := range facts {
			out = append(out, ohRow(f))
		}
		return out
	}
	return []ohFactCol{
		{Kind: "IPV4", Proto: o.V4Proto, Side: "left", Rows: rows(o.V4)},
		{Kind: "IPV6", Proto: o.V6Proto, Side: "right", Rows: rows(o.V6)},
	}
}

// renderToHTML renders one widget to a fragment for injection into a composing
// template (the overview places the chart and the three listings itself).
func renderToHTML(r *Renderer, w interface {
	renderInto(*Renderer, io.Writer, string) error
}, csrf string) (template.HTML, error) {
	var b strings.Builder
	if err := w.renderInto(r, &b, csrf); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// overviewInterfaces is the ports listing as a flat table: the port (mono, greyed
// when down), its link state as a status dot + word (the WAN uplink tagged), the
// negotiated speed, and today's RX/TX.
func overviewInterfaces() *Table {
	up := func(port, speed, rx string, wan bool) TableRow {
		link := TableCell{Text: "Up", Variant: "success"}
		if wan {
			link.Tag, link.TagVariant = "WAN", "info"
		}
		return TableRow{Cells: []TableCell{
			{Text: port}, link, {Text: speed, Muted: true}, {Text: rx},
		}}
	}
	return &Table{
		Style: "flat", Title: "Interfaces", Detail: "5 ports",
		Columns: []TableColumn{
			{Label: "Port", Kind: "mono"}, {Label: "Link", Kind: "status"},
			{Label: "Speed", Kind: "text"}, {Label: "RX / TX today", Kind: "num"},
		},
		Rows: []TableRow{
			{Cells: []TableCell{
				{Text: "eth0", Muted: true}, {Text: "No link", Variant: "neutral"},
				{Text: "—", Muted: true}, {Text: "—"},
			}},
			up("eth1", "1 Gbps · full", "4.1 / 0.9 GB", false),
			up("eth2", "1 Gbps · full", "0.3 / 0.1 GB", false),
			up("eth3", "10 Gbps", "2.7 / 1.2 GB", false),
			up("eth4", "10 Gbps", "18.4 / 2.1 GB", true),
		},
	}
}

// overviewLeases is the DHCP lease table: the device with its zone chip, its MAC
// and IP as copy-pastable machine strings, and time remaining.
func overviewLeases() *Table {
	row := func(dev, zone, mac, ip, expires string) TableRow {
		return TableRow{Cells: []TableCell{
			{Text: dev, Chip: zone},
			{Text: mac, Copy: true},
			{Text: ip, Copy: true},
			{Text: expires},
		}}
	}
	return &Table{
		Style: "flat", Title: "DHCP leases", Detail: "5 active",
		Columns: []TableColumn{
			{Label: "Device", Kind: "name"}, {Label: "MAC", Kind: "mono"},
			{Label: "IP", Kind: "mono"}, {Label: "Expires", Kind: "num"},
		},
		Rows: []TableRow{
			row("Gaming PC", "lan", "a4:83:e7:2b:19:0c", "192.168.1.104", "11h 12m"),
			row("Living Room TV", "lan", "dc:a6:32:44:8f:2e", "192.168.1.140", "9h 03m"),
			row("family-laptop", "lan", "f0:18:98:1d:77:a1", "192.168.1.156", "6h 44m"),
			row("Alice's iPhone", "lan", "b8:e6:0c:5a:3f:d2", "192.168.1.181", "2h 20m"),
			row("Unknown device", "guest", "9e:2f:11:c4:08:5b", "192.168.20.44", "30m"),
		},
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
	"strconv"
	"strings"
)

// Overview is the advanced overview page, transferred hardcoded from the design:
// a verdict line with a Basic/Advanced toggle, a strip of status tiles, the
// IPv4/IPv6 connection facts, the internet-traffic graph, the System panel, and
// the Interfaces listing. It is the shell's own page content — not part of the
// plugin-facing vocabulary, so it never appears in Decode — a faithful transfer
// that later steps wire to live data.
//
// The graph reuses the generic Chart widget and the listing reuses the generic
// flat Table widget (both injected as rendered HTML); Overview owns only the page
// composition — the verdict, the tiles, the facts, and the section rhythm. The
// verdict headline is set in the serif display face (Fraunces).
//
// Firmware, Kernel, and Uptime are the live System facts, and DownSeries/UpSeries
// (with DownVal/UpVal, the newest values) the live WAN traffic minute — all filled
// by the shell from the backend before rendering; the rest is placeholder pending
// its own wiring.
type Overview struct {
	Firmware    string
	Kernel      string
	Uptime      string
	WANKnown    bool
	WANUp       bool
	WANDevice   string
	WANUptime   string
	WiFiPresent bool

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
	// TempDot is the temperature status tone ("success"|"warning"|"danger").
	Model         string
	Temperature   string
	TempDot       string
	Fan           string
	Power         string
	SensorSummary string

	// Interfaces is the live listing: every kernel interface, enriched with its
	// topology/UCI meaning.
	Interfaces []OverviewInterface

	// DevicesOnline is how many devices are on the network right now, and
	// DevicesKnown whether the box could count them at all. The roster itself is
	// the Devices page, which DevicesHref names; the tile is this page's doorway
	// to it and states the one number the strip has room for. An uncounted
	// network draws no tile — the same silence a box without Wi-Fi keeps.
	DevicesOnline int
	DevicesKnown  bool
	DevicesHref   string

	// The strip's remaining doorways. SecurityHref is the firewall's page while
	// that plugin answers and empty while it does not — the tile is then a plain
	// status, since a door to a page nobody serves is worse than none.
	// SoftwareHref is the shell's own package surface, which always answers.
	SecurityHref string
	SoftwareHref string

	// What this router knows about what it could install. UpdatesKnown is whether
	// a check has ever completed — before one has, the tile makes no claim either
	// way. UpdatesPackages counts the packages with newer versions in the feeds,
	// UpdatesFirmware says a newer system build is on offer, UpdatesCheckedAgo is
	// how old that answer is, and UpdatesHref is where a person acts on it.
	UpdatesKnown      bool
	UpdatesPackages   int
	UpdatesFirmware   bool
	UpdatesCheckedAgo string
	UpdatesHref       string
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

// children is empty: the overview composes its chart, meters, and tables at
// render time, so its renderInto runs the localization walk over each itself.
func (*Overview) children() []Widget { return nil }

// ohTile is one status tile in the strip under the verdict: an eyebrow label,
// a trailing icon, a status dot + word, and a caption. Page-owned chrome (like
// the traffic legend), not a widget: its anatomy — the dot beside the word,
// the quiet caption — is this page's own. Variant speaks the tone vocabulary.
// A tile with an Href is also a doorway: the whole tile becomes the link.
type ohTile struct {
	Label   string
	Icon    string
	Variant string // "success" | "warning"
	Status  string
	Caption string
	Key     string // live hook: the stream refreshes the caption in place
	Href    string
}

// ohFactColView is one connection-facts column (IPv4 or IPv6): an eyebrow
// (kind + protocol) over its rows — a Properties widget rendered to HTML. Side
// places it left or right of the divider.
type ohFactColView struct {
	Kind  string
	Proto string
	Side  string // "left" | "right"
	Rows  template.HTML
}

// overviewMastheadView is the top block's render model — the verdict, the view
// switch, the status tiles, and the connection facts — shared by the home page
// and the overview-preview styleguide workbench, so the block cannot drift.
type overviewMastheadView struct {
	Kicker string
	Lead   string
	Accent string
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

	SysMeta  string
	Metrics  []template.HTML
	SysLeft  template.HTML
	SysRight template.HTML

	Interfaces template.HTML
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
		Size:      "panel",
		Axis:      true,
		Unit:      "Mbps",
		AxisStart: "60 seconds ago",
		AxisEnd:   "now",
		Label:     "Internet traffic — download and upload, last minute",
		Series: []ChartSeries{
			{Label: "down", Role: "sky", Fill: true, Values: down},
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
	interfacesTable := o.interfacesTable()
	r.translate(interfacesTable)
	interfaces, err := renderToHTML(r, interfacesTable, csrf)
	if err != nil {
		return err
	}

	// The System headline metrics as bar-layout Meters — each a fixed accent
	// (icon + bar), not a health band, so the row reads as a dashboard.
	metrics := make([]template.HTML, 0, 4)
	for _, mt := range o.sysMeters() {
		r.translate(mt)
		html, err := renderToHTML(r, mt, csrf)
		if err != nil {
			return err
		}
		metrics = append(metrics, html)
	}

	// The verdict and tiles are this template's own render model, not walkable
	// widget structs, so their prose is localized here at the site.
	tiles := []ohTile{o.internetTile(r.tr)}
	if o.WiFiPresent {
		tiles = append(tiles, ohTile{Label: r.tr("WI-FI"), Icon: "wifi", Variant: "success", Status: r.tr("Both bands active"), Caption: r.tr("2.4 & 5 GHz")})
	}
	if o.DevicesKnown {
		tiles = append(tiles, o.devicesTile(r.tr))
	}
	tiles = append(tiles,
		ohTile{Label: r.tr("SECURITY"), Icon: "shield", Variant: "success", Status: r.tr("Protected"),
			Caption: r.tr("Firewall on"), Href: o.SecurityHref},
		o.softwareTile(r.tr),
	)

	facts, err := o.factCols(r, csrf)
	if err != nil {
		return err
	}
	sysLeft, err := o.renderWidget(r, &Properties{Style: "system", Items: []Property{
		sysProp("Model", o.Model, false),
		sysProp("Firmware", o.Firmware, true),
		sysProp("Kernel", o.Kernel, true),
		sysProp("Uptime", o.Uptime, false),
	}}, csrf)
	if err != nil {
		return err
	}
	sysRight, err := o.renderWidget(r, o.sysRight(), csrf)
	if err != nil {
		return err
	}

	chartMeta := r.tr("live") + " · WAN"
	if o.WANDevice != "" {
		chartMeta = r.tr("live") + " · " + o.WANDevice
	}
	v := overviewView{
		Masthead: overviewMastheadView{
			Kicker: r.tr("ALL GOOD"),
			Lead:   r.tr("Your network is "),
			Accent: r.tr("healthy"),
			Tiles:  tiles,
			Facts:  facts,
		},
		ChartTitle:  r.tr("Internet traffic"),
		ChartMeta:   chartMeta,
		DownVal:     o.DownVal,
		UpVal:       o.UpVal,
		RateUnit:    "Mbps",
		Chart:       chartHTML,
		TrafficSeed: string(seed),

		SysMeta:  r.tr("hardware · live"),
		Metrics:  metrics,
		SysLeft:  sysLeft,
		SysRight: sysRight,

		Interfaces: interfaces,
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
	tile := ohTile{Label: tr("INTERNET"), Icon: "globe", Variant: "warning", Status: tr("Unavailable"), Key: "internet-uptime"}
	if !o.WANKnown {
		return tile
	}
	if !o.WANUp {
		tile.Status, tile.Caption = tr("Not connected"), tr("WAN is down")
		return tile
	}
	tile.Variant, tile.Status = "success", tr("Connected")
	if o.WANUptime != "" {
		tile.Caption = tr("for ") + o.WANUptime
	}
	return tile
}

// devicesTile is the strip's doorway to the roster: how many devices are on the
// network right now, and a way in to see which. The count is the status — the
// number is the reason a person looks at this tile.
func (o *Overview) devicesTile(tr func(string) string) ohTile {
	status := tr("Nobody connected")
	if o.DevicesOnline == 1 {
		status = tr("1 device online")
	} else if o.DevicesOnline > 1 {
		status = strconv.Itoa(o.DevicesOnline) + tr(" devices online")
	}
	return ohTile{
		Label: tr("DEVICES"), Icon: "devices", Variant: "success",
		Status: status, Caption: tr("See who is here"), Href: o.DevicesHref,
	}
}

// softwareTile is the strip's software fact and its doorway. A router that has
// never checked claims nothing about updates and leads to its software instead; a
// router with something to install says so in amber and leads to the page that
// installs it. The tile never invents an "up to date" it has not verified.
func (o *Overview) softwareTile(tr func(string) string) ohTile {
	tile := ohTile{Label: tr("SOFTWARE"), Icon: "download", Variant: "success", Href: o.SoftwareHref}
	switch {
	case !o.UpdatesKnown:
		tile.Status, tile.Caption = tr("Installed software"), tr("Packages and firmware")
	case o.UpdatesFirmware:
		tile.Variant, tile.Status = "warning", tr("Update available")
		tile.Caption, tile.Href = tr("A newer system build"), o.UpdatesHref
	case o.UpdatesPackages > 0:
		tile.Variant, tile.Status = "warning", tr("Update available")
		tile.Caption, tile.Href = updatesReadyCaption(tr, o.UpdatesPackages), o.UpdatesHref
	default:
		tile.Status, tile.Caption = tr("Up to date"), tr("checked ")+o.UpdatesCheckedAgo
	}
	return tile
}

func updatesReadyCaption(tr func(string) string, packages int) string {
	if packages == 1 {
		return tr("1 package ready")
	}
	return strconv.Itoa(packages) + tr(" packages ready")
}

// sysMeters builds the System gauges from the live fields, each a named bar meter
// so the stream can update it in place.
func (o *Overview) sysMeters() []*Meter {
	out := make([]*Meter, 0, len(o.SysMetrics))
	for _, m := range o.SysMetrics {
		out = append(out, &Meter{
			Name: m.Name, Icon: m.Icon, Role: m.Role,
			Label: m.Label, Value: m.Value, Unit: m.Unit, Fill: m.Fill, Detail: m.Detail,
		})
	}
	return out
}

// sysRight builds the hardware-sensor fact sheet, omitting any reading the box
// doesn't expose — a PC with no power sensor simply shows no "Power draw" row,
// never a fabricated zero. Keys tag the live rows for the overview stream.
func (o *Overview) sysRight() *Properties {
	rows := make([]Property, 0, 4)
	if o.Temperature != "" {
		rows = append(rows, Property{Label: "Temperature", Value: o.Temperature, Verbatim: true, Dot: o.TempDot, Key: "temperature"})
	}
	if o.Fan != "" {
		rows = append(rows, Property{Label: "Fan", Value: o.Fan, Verbatim: true, Key: "fan"})
	}
	if o.Power != "" {
		rows = append(rows, Property{Label: "Power draw", Value: o.Power, Verbatim: true, Key: "power"})
	}
	if o.SensorSummary != "" {
		rows = append(rows, Property{Label: "Sensors", Value: o.SensorSummary, Verbatim: true, Key: "summary"})
	}
	return &Properties{Style: "system", Items: rows}
}

// factCols builds the IPv4/IPv6 connection-facts columns from the live fields:
// each side an eyebrow over a left-aligned Properties sheet (mono, copyable —
// the reading order for addresses a person compares line by line). The panel is
// the advanced face of the internet tile above it (ADR-015 §2), so a page that
// carries neither side draws no columns at all.
func (o *Overview) factCols(r *Renderer, csrf string) ([]ohFactColView, error) {
	if len(o.V4) == 0 && len(o.V6) == 0 {
		return nil, nil
	}
	sheet := func(facts []OverviewFact) *Properties {
		items := make([]Property, 0, len(facts))
		for _, f := range facts {
			items = append(items, Property{Label: f.Label, Value: f.Value, Mono: true, Emphasis: true, Copy: f.Copy})
		}
		return &Properties{Align: "left", Items: items}
	}
	v4, err := o.renderWidget(r, sheet(o.V4), csrf)
	if err != nil {
		return nil, err
	}
	v6, err := o.renderWidget(r, sheet(o.V6), csrf)
	if err != nil {
		return nil, err
	}
	return []ohFactColView{
		{Kind: "IPV4", Proto: o.V4Proto, Side: "left", Rows: v4},
		{Kind: "IPV6", Proto: o.V6Proto, Side: "right", Rows: v6},
	}, nil
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

// interfacesTable is the kernel's complete network topology: physical ports,
// bridges, VLANs and tunnels share one listing, enriched with UCI meaning.
func (o *Overview) interfacesTable() *Table {
	rows := make([]TableRow, 0, len(o.Interfaces))
	for _, n := range o.Interfaces {
		name := TableCell{Text: n.Name}
		if n.Physical {
			name.Chips = append(name.Chips, TableChip{Icon: "ethernet-port", Label: "port"})
		}
		if n.Kind == "wifi" {
			name.Chips = append(name.Chips, TableChip{Icon: "wifi", Label: "Wi-Fi"})
		}
		if n.Zone != "" {
			name.Chips = append(name.Chips, TableChip{Icon: "zone", Label: n.Zone})
		}
		if n.WAN {
			name.Tag, name.TagVariant, name.TagIcon = "WAN", "info", "globe"
		}
		rows = append(rows, TableRow{
			Key: "interface:" + n.Name,
			Cells: []TableCell{
				name,
				{Text: interfaceKindLabel(n.Kind)},
				interfaceStateCell(n, "state"),
				{Chips: interfaceRelationChips(n.Relations)},
				{Text: n.RxRate, Key: "rx-rate"},
				{Text: n.TxRate, Key: "tx-rate"},
			},
			Drawer: o.interfaceDrawer(n),
		})
	}
	return &Table{
		Style: "flat", Align: "top", Title: "Interfaces", Detail: countLabel(len(rows), "interface"),
		Columns: []TableColumn{
			{Label: "Interface", Kind: "name"}, {Label: "Type", Kind: "keyword"},
			{Label: "State", Kind: "status"}, {Label: "Topology", Kind: "entity"},
			{Label: "RX", Kind: "rate"}, {Label: "TX", Kind: "rate"},
		},
		Rows: rows,
	}
}

func interfaceStateCell(n OverviewInterface, key string) TableCell {
	state := n.State
	if state == "" {
		state = "unknown"
	}
	cell := TableCell{Text: strings.ToUpper(state[:1]) + state[1:], Variant: "neutral", Key: key}
	if state == "up" {
		cell.Variant = "success"
	}
	return cell
}

func interfaceRelationChips(relations []OverviewInterfaceRelation) []TableChip {
	out := make([]TableChip, 0, len(relations))
	for _, relation := range relations {
		icon := "network"
		if relation.Physical {
			icon = "ethernet-port"
		}
		out = append(out, TableChip{Icon: icon, Label: relation.Name})
	}
	return out
}

func interfaceKindLabel(kind string) string {
	switch kind {
	case "port":
		return "Ethernet"
	case "wifi":
		return "Wi-Fi"
	case "vlan":
		return "VLAN"
	case "pppoe":
		return "PPPoE"
	case "tunnel":
		return "Tunnel"
	case "loopback":
		return "Loopback"
	case "bridge":
		return "Bridge"
	default:
		return "Virtual"
	}
}

// interfaceDrawer carries the complete counters and UCI facts without widening
// the main topology table.
func (o *Overview) interfaceDrawer(n OverviewInterface) *RowDrawer {
	// Every value but Type is data — an operstate, an identity, a rate — and
	// declares itself Verbatim; only the kind label is words.
	facts := &Properties{Items: []Property{
		{Label: "Type", Value: interfaceKindLabel(n.Kind)},
		{Label: "State", Value: orDash(n.State), Verbatim: true},
		{Label: "Network", Value: orDash(strings.Join(n.Networks, ", ")), Verbatim: true},
		{Label: "Role", Value: map[bool]string{true: "WAN", false: "—"}[n.WAN], Verbatim: true},
		{Label: "Protocol", Value: orDash(n.Proto), Verbatim: true},
		{Label: "Subnet", Value: orDash(n.Subnet), Verbatim: true},
		{Label: "Zone", Value: orDash(n.Zone), Verbatim: true},
		{Label: "VLAN", Value: orDash(n.VLAN), Verbatim: true},
		{Label: "RX rate", Value: n.RxRate, Verbatim: true},
		{Label: "TX rate", Value: n.TxRate, Verbatim: true},
		{Label: "RX packets", Value: n.RxPackets, Verbatim: true},
		{Label: "TX packets", Value: n.TxPackets, Verbatim: true},
		{Label: "RX total", Value: n.RxTotal, Verbatim: true},
		{Label: "TX total", Value: n.TxTotal, Verbatim: true},
	}}
	children := []Widget{facts}
	if len(n.Relations) != 0 {
		topology := &Table{
			Style: "flat", Condensed: true,
			Columns: []TableColumn{{Label: "Related interface", Kind: "entity"}},
		}
		for _, relation := range n.Relations {
			topology.Rows = append(topology.Rows, TableRow{Cells: []TableCell{{Chips: interfaceRelationChips([]OverviewInterfaceRelation{relation})}}})
		}
		children = append([]Widget{topology}, children...)
	}
	return &RowDrawer{Title: n.Name, Verbatim: true, Size: "wide", Children: children}
}

func zoneChip(name string) []TableChip {
	if name == "" {
		return nil
	}
	return []TableChip{{Icon: "zone", Label: name}}
}

// orDash falls a missing cell value back to a quiet dash.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// countLabel renders a header count like "5 ports" / "1 lease", pluralising the
// noun with a plain "s".
func countLabel(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

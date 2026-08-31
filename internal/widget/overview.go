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
// a verdict line with a Basic/Advanced toggle, a strip of four status tiles, the
// IPv4/IPv6 connection facts, the internet-traffic graph, the System panel, and
// the Interfaces and Connected-devices listings. It is the shell's own
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

	// Interfaces and Devices are the live listings — every kernel interface,
	// enriched with its topology/UCI meaning, and the clients on those networks.
	Interfaces []OverviewInterface
	Devices    []OverviewDevice
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

// OverviewDevice is one Connected-devices row and its drawer: the device name,
// its MAC, its primary v4/v6 addresses (the stacked cell), the Interface (logical
// network) it sits on, and presence ("online"|"idle"|"offline") — plus the fuller
// story the Details drawer opens: every address, its zone, connection, lease, and
// traffic. Interface ties the row back to the Interfaces table; Zone rides the
// drawer, not the row (the interface already names the segment).
type OverviewDevice struct {
	Name      string
	Icon      string // device-type Lucide glyph, resolved deterministically from MAC OUI / hostname (internal/deviceicon)
	MAC       string
	V4        string
	V6        string
	Interface string
	Presence  string

	DUID       string
	Zone       string
	Addresses  []OverviewAddr
	Connection string
	Lease      string
	Traffic    string
	Conns      string
}

// OverviewAddr is one row of a device's full address list in the drawer: the
// address, its family ("IPv4"|"IPv6"), and the kernel's confidence in it.
type OverviewAddr struct {
	Addr   string
	Family string
	State  string
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

// ohTile is one status tile: an eyebrow label, a trailing icon, a status dot +
// word, and a caption. Variant "success" tints emerald; "warning" tints amber.
type ohTile struct {
	Label   string
	Icon    string
	Variant string // the tone vocabulary: "success" | "warning"
	Status  string
	Caption string
	Key     string
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
// with an optional leading status dot (Dot speaks the tone vocabulary). Key
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
	Kicker  string
	Lead    string
	Accent  string
	Tiles   []ohTile
	HasWiFi bool
	Facts   []ohFactCol

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
	Devices    template.HTML
}

// orUnavailable falls a missing live fact back to a localized "unavailable" so the
// System panel degrades honestly rather than showing a stale placeholder.
func orUnavailable(tr func(string) string, s string) string {
	if s == "" {
		return tr("unavailable")
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
	devicesTable := o.devicesTable()
	r.translate(devicesTable)
	devices, err := renderToHTML(r, devicesTable, csrf)
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

	// The verdict, tiles, and System facts are this template's own render model,
	// not walkable widget structs, so their prose is localized here at the site.
	tiles := []ohTile{o.internetTile(r.tr)}
	if o.WiFiPresent {
		tiles = append(tiles, ohTile{Label: r.tr("WI-FI"), Icon: "wifi", Variant: "success", Status: r.tr("Both bands active"), Caption: r.tr("2.4 & 5 GHz")})
	}
	tiles = append(tiles,
		ohTile{Label: r.tr("SECURITY"), Icon: "shield", Variant: "success", Status: r.tr("Protected"), Caption: r.tr("Firewall on")},
		ohTile{Label: r.tr("SOFTWARE"), Icon: "download", Variant: "warning", Status: r.tr("Update available"), Caption: r.tr("Security fixes")},
	)

	chartMeta := r.tr("live") + " · WAN"
	if o.WANDevice != "" {
		chartMeta = r.tr("live") + " · " + o.WANDevice
	}
	v := overviewView{
		Kicker:      r.tr("ALL GOOD"),
		Lead:        r.tr("Your network is "),
		Accent:      r.tr("healthy"),
		Tiles:       tiles,
		HasWiFi:     o.WiFiPresent,
		Facts:       o.factCols(r.tr),
		ChartTitle:  r.tr("Internet traffic"),
		ChartMeta:   chartMeta,
		DownVal:     o.DownVal,
		UpVal:       o.UpVal,
		RateUnit:    "Mbps",
		Chart:       chartHTML,
		TrafficSeed: string(seed),

		SysMeta: r.tr("hardware · live"),
		Metrics: metrics,
		SysLeft: []ohProp{
			{Label: r.tr("Model"), Value: orUnavailable(r.tr, o.Model)},
			{Label: r.tr("Firmware"), Value: orUnavailable(r.tr, o.Firmware), Mono: true},
			{Label: r.tr("Kernel"), Value: orUnavailable(r.tr, o.Kernel), Mono: true},
			{Label: r.tr("Uptime"), Value: orUnavailable(r.tr, o.Uptime)},
		},
		SysRight: o.sysRight(r.tr),

		Interfaces: interfaces,
		Devices:    devices,
	}
	return r.execute(out, "overview.html.tmpl", v)
}

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
func (o *Overview) sysRight(tr func(string) string) []ohProp {
	rows := make([]ohProp, 0, 4)
	if o.Temperature != "" {
		rows = append(rows, ohProp{Label: tr("Temperature"), Value: o.Temperature, Dot: o.TempDot, Key: "temperature"})
	}
	if o.Fan != "" {
		rows = append(rows, ohProp{Label: tr("Fan"), Value: o.Fan, Key: "fan"})
	}
	if o.Power != "" {
		rows = append(rows, ohProp{Label: tr("Power draw"), Value: o.Power, Key: "power"})
	}
	if o.SensorSummary != "" {
		rows = append(rows, ohProp{Label: tr("Sensors"), Value: o.SensorSummary, Key: "summary"})
	}
	return rows
}

// factCols builds the IPv4/IPv6 connection-facts columns from the live fields. The
// row labels and any prose value ("Not configured") are localized; an address value
// simply misses the catalog and stays verbatim.
func (o *Overview) factCols(tr func(string) string) []ohFactCol {
	rows := func(facts []OverviewFact) []ohRow {
		out := make([]ohRow, 0, len(facts))
		for _, f := range facts {
			out = append(out, ohRow{Label: tr(f.Label), Value: tr(f.Value), Copy: f.Copy})
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

// devicesTable is the Connected-devices roster: one row per device the box has
// seen — a device-type icon + name + zone chip, MAC, the IPv4 address, and
// presence. The row carries only the IPv4; each row's Details opens a drawer
// with the full address list (both families) and the rest of the story. This is
// the unified view (all devices, both families, DHCP or not), not a lease dump.
func (o *Overview) devicesTable() *Table {
	rows := make([]TableRow, 0, len(o.Devices))
	for _, d := range o.Devices {
		rows = append(rows, TableRow{
			Cells: []TableCell{
				{Text: d.Name, LeadIcon: d.Icon},
				{Text: d.Interface, Chips: zoneChip(d.Zone)},
				{Text: d.MAC, Copy: true, Emphasis: true},
				{Text: d.V4, Copy: true, Emphasis: true},
				presenceCell(d.Presence),
			},
			Drawer: o.deviceDrawer(d),
		})
	}
	return &Table{
		Style: "flat", Align: "top", Title: "Connected devices", Detail: countLabel(len(rows), "device"),
		Columns: []TableColumn{
			{Label: "Device", Kind: "name"}, {Label: "Interface", Kind: "reference"},
			{Label: "MAC", Kind: "mono"}, {Label: "IPv4", Kind: "addr"},
			{Label: "Status", Kind: "status"},
		},
		Rows: rows,
	}
}

// presenceCell renders a device's presence as a status dot + word: online reads
// emerald, idle amber, offline a quiet grey.
func presenceCell(p string) TableCell {
	switch p {
	case "online":
		return TableCell{Text: "Online", Variant: "success"}
	case "idle":
		return TableCell{Text: "Idle", Variant: "warning"}
	default:
		return TableCell{Text: "Offline", Variant: "neutral"}
	}
}

// deviceDrawer builds the Details panel for one device: its full address list and
// a facts block (connection, DHCP lease, traffic) — the more/all-info view behind
// the row.
func (o *Overview) deviceDrawer(d OverviewDevice) *RowDrawer {
	addrRows := make([]TableRow, 0, len(d.Addresses))
	for _, a := range d.Addresses {
		addrRows = append(addrRows, TableRow{Cells: []TableCell{
			{Text: a.Addr, Copy: true, Emphasis: true},
			{Text: a.Family, Variant: family(a.Family)},
			{Text: orDash(a.State), Muted: true},
		}})
	}
	addresses := &Table{
		Style: "flat", Condensed: true,
		Columns: []TableColumn{
			{Label: "Address", Kind: "mono"}, {Label: "Family", Kind: "pill"},
			{Label: "State", Kind: "text"},
		},
		Rows: addrRows,
	}

	facts := &Table{
		Style: "flat", Condensed: true,
		Columns: []TableColumn{{Kind: "keyword"}, {Kind: "text"}},
		Rows: []TableRow{
			factRow("MAC", d.MAC),
			factRow("DUID", orDash(d.DUID)),
			factRow("Interface", orDash(d.Interface)),
			factRow("Zone", orDash(d.Zone)),
			factRow("Connection", orDash(d.Connection)),
			factRow("DHCP lease", orDash(d.Lease)),
			factRow("Traffic", orDash(d.Traffic)),
			factRow("Connections", orDash(d.Conns)),
		},
	}

	return &RowDrawer{Title: d.Name, Size: "wide", Children: []Widget{addresses, facts}}
}

// factRow is one label/value line for a device's Details facts table.
func factRow(label, value string) TableRow {
	return TableRow{Cells: []TableCell{{Text: label}, {Text: value}}}
}

// family maps an address family to the pill palette — v6 the calmer tint.
func family(fam string) string {
	if fam == "IPv6" {
		return "info"
	}
	return "neutral"
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
	facts := &Table{
		Style: "flat", Condensed: true,
		Columns: []TableColumn{{Kind: "keyword"}, {Kind: "text"}},
		Rows: []TableRow{
			factRow("Type", interfaceKindLabel(n.Kind)),
			factRow("State", orDash(n.State)),
			factRow("Network", orDash(strings.Join(n.Networks, ", "))),
			factRow("Role", map[bool]string{true: "WAN", false: "—"}[n.WAN]),
			factRow("Protocol", orDash(n.Proto)),
			factRow("Subnet", orDash(n.Subnet)),
			factRow("Zone", orDash(n.Zone)),
			factRow("VLAN", orDash(n.VLAN)),
			factRow("RX rate", n.RxRate),
			factRow("TX rate", n.TxRate),
			factRow("RX packets", n.RxPackets),
			factRow("TX packets", n.TxPackets),
			factRow("RX total", n.RxTotal),
			factRow("TX total", n.TxTotal),
		},
	}
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
	return &RowDrawer{Title: n.Name, Size: "wide", Children: children}
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

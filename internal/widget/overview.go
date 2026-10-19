// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"io"
	"slices"
	"strings"
)

// Overview composes the shell's landing page from observed router state: a
// verdict over cards that each hold one kind of reading (ADR-022). It is
// outside the plugin vocabulary; the cards' bodies reuse the existing
// renderers.
type Overview struct {
	// Clock and Zone are the router's time and its zone, for the System
	// card's Local time.
	Clock, Zone     string
	InterfacesKnown bool
	// Doors are where a card opens into a plugin, by the kind of thing it
	// opens ("network", "tunnel"): the running plugin's claim on that kind's
	// door. A kind no plugin claims has none, and its card no Go link.
	Doors map[string]OverviewDoor

	// Layout is where the cards stand, read from the browser (ADR-022); the
	// zero value is the default layout.
	Layout OverviewLayout
	// Contributed are the plugins' cards this render drew (ADR-026), and Kept
	// the ids of installed plugins' cards that did not, whose places hold.
	Contributed []OverviewContribution
	Kept        []string

	Firmware string
	Kernel   string
	// Uptime is how long the router has run, and WANUptime how long the
	// uplink has held, in seconds; nil is not known.
	Uptime    *int64
	WANKnown  bool
	WANUp     bool
	WANDevice string
	WANUptime *int64
	// WANAddress is the uplink's IPv4 address with its prefix
	// ("203.0.113.7/32"), the address the internet sees.
	WANAddress string

	// DownSeries and UpSeries are the uplink's live minute in Mbit/s, and
	// DownNow and UpNow the newest of each.
	DownSeries     []float64
	UpSeries       []float64
	DownNow, UpNow float64
	// TrafficRangeURL offers the Internet graph's window where history
	// stands behind it (Traffic.RangeURL).
	TrafficRangeURL string
	// TrafficWithheld is why the graph offers no past window though history
	// stands behind it, said under the graph; empty while one is offered.
	TrafficWithheld string

	// The IPv4/IPv6 connection facts. Each side carries its protocol label and
	// its rows; an absent address family says so in its place.
	V4Proto string
	V4      []OverviewFact
	V6Proto string
	V6      []OverviewFact

	// SysMetrics are the live System gauges (load, CPU, memory, storage); empty
	// renders an unavailable state.
	SysMetrics []OverviewMeter

	// Hardware is the board profile's readings, formatted; nil where none
	// answered or the card is hidden.
	Hardware *OverviewSensors

	// Model is the board's human name; Arch the package architecture its
	// firmware was built for, Target the OpenWrt target/subtarget.
	Model, Arch, Target string

	// Interfaces is the live listing: every kernel interface, enriched with its
	// topology/UCI meaning.
	Interfaces []OverviewInterface
	// Panel is the router's panel that opens the Ports card, its ports in
	// their place on the case; nil on a box with no ports.
	Panel *Ports

	// The Ports card's line includes the observed device count when available.
	DevicesOnline int
	DevicesKnown  bool
	// DevicesWithheld is why the line gives no count where the session may
	// not see the devices, said in the count's place.
	DevicesWithheld string

	// FirewallState comes from fw4's loaded kernel table, not its configuration.
	// Empty means the runtime read failed. No card shows it; the verdict counts
	// a firewall that is not fully loaded.
	FirewallState string

	// Exposed is that the firewall's config lets the internet reach this web
	// interface (System › Access says why). No card shows it; the verdict
	// counts it.
	Exposed bool

	// FailedServices names, sorted, the services procd stopped restarting.
	// The System card owns them: its header names them, its facts list them,
	// and it opens the Services page while there are any.
	FailedServices []string
}

// OverviewInterface is one kernel interface enriched with runtime topology and
// any matching UCI network meaning.
type OverviewInterface struct {
	Name     string
	Kind     string
	State    string
	Physical bool
	// Conduit marks a DSA conduit: physical, but the switch's link to the
	// SoC, not a port on the case.
	Conduit   bool
	WAN       bool
	Networks  []string
	Relations []OverviewInterfaceRelation
	VLAN      string
	Subnet    string
	Zone      string
	Proto     string
	// RxBPS and TxBPS are what moves on it now, in bits per second.
	RxBPS, TxBPS uint64
}

// inUse is whether anything asked for the port: a network sits on it, or it is
// a bridge's member or a VLAN's parent. A spare port is neither, so its missing
// cable is nobody's concern.
func (n OverviewInterface) inUse() bool {
	return len(n.Networks) > 0 || len(n.Relations) > 0
}

// port is whether the interface is a socket on the case: physical and no DSA
// conduit.
func (n OverviewInterface) port() bool {
	return n.Physical && !n.Conduit
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
	// Duration makes the fact a stretch of time, in seconds (a lease's term).
	Duration *int64
}

// OverviewMeter is one live System gauge — a named meter with its health
// band, value, fill, the share past which it warns, and caption. The Name
// lets the overview stream update it in place.
type OverviewMeter struct {
	Name   string
	Label  string
	Band   string
	Value  string
	Unit   string
	Detail string
	Fill   int
	Warn   int
}

// InternetTraffic is the overview's Internet graph of the uplink device, in
// the Traffic card: the live minute when window is empty or "minute", else the
// past window drawn from history, its figures the window's averages. Rates are
// in Mbit/s.
func InternetTraffic(downNow, upNow float64, down, up []float64, window string) *Traffic {
	label := "Internet traffic — download and upload, last minute"
	if window != "" && window != TrafficRanges[0].Value {
		label = "Internet traffic — download and upload"
	}
	return &Traffic{
		Live: "wan", Label: label, ID: InternetTrafficID,
		DownNow: downNow, UpNow: upNow, Down: down, Up: up,
		Range: window, Place: PlaceCompact,
	}
}

// InternetTrafficID addresses the Overview's Internet graph, so the window
// fetched for it lands in its place.
const InternetTrafficID = "overview-traffic-graph"

// wanSeriesFallback is the flat-line length drawn before any WAN sample lands.
const wanSeriesFallback = 60

func (*Overview) isWidget() {}

// children is empty: the overview composes its cards at render time, so its
// renderInto runs the localization walk over each widget itself.
func (*Overview) children() []Widget { return nil }

// overviewVerdictView is the page's one sentence.
type overviewVerdictView struct {
	Tone, Lead, Accent, Tail string
}

// overviewCardView is one card as the page draws it.
type overviewCardView struct {
	ID, Title, Icon string
	Head            cardHead
	Closed          bool
	Href, OpenLabel string
	Body            template.HTML
	// Kept is a plugin's card that did not draw this time: only its place.
	Kept bool
}

// overviewShelfView is a hidden card on the Arrange shelf, wearing its tone.
// A kept one holds a hidden card's place for a plugin card that did not draw.
type overviewShelfView struct {
	ID, Title, Tone string
	Kept            bool
}

// overviewView is the page's layout model: the verdict, the two columns of
// cards in the browser's order, and the hidden cards. Blank is the chip with
// nothing in it, which the page clones when it hides a card in place.
type overviewView struct {
	Verdict overviewVerdictView
	Cols    [2][]overviewCardView
	Hidden  []overviewShelfView
	Blank   overviewShelfView
}

// OverviewContribution is a card a plugin contributes to the Overview
// (ADR-026), as its plugin answered it: already in its own plugin's words,
// so no pass of the shell's translates it again. Href is its door, empty for
// none; Body is drawn in the card's body like a shell card's.
type OverviewContribution struct {
	ID, Title, Icon              string
	Tone, Status, Summary, Cause string
	Href                         string
	Body                         Widget
}

// OverviewDoor is a card's door into a plugin: where the claiming plugin
// lists the kind, and its word for that page, already in its own language,
// so no pass of the shell's translates it again.
type OverviewDoor struct {
	Href, Word string
}

// head is the contributed card's header as the shell's cards carry theirs.
func (c OverviewContribution) head() cardHead {
	return cardHead{Tone: c.Tone, Status: c.Status, Summary: c.Summary, Cause: c.Cause}
}

// layout is the browser's layout, or the default where none was read.
func (o *Overview) layout() OverviewLayout {
	if len(o.Layout.Cols[0])+len(o.Layout.Cols[1]) == 0 {
		return DefaultOverviewLayout()
	}
	return o.Layout
}

// contributed is the plugin card of that id this render drew.
func (o *Overview) contributed(id string) (OverviewContribution, bool) {
	i := slices.IndexFunc(o.Contributed, func(c OverviewContribution) bool { return c.ID == id })
	if i < 0 {
		return OverviewContribution{}, false
	}
	return o.Contributed[i], true
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
			var card overviewCardView
			var err error
			spec, shell := specs[id]
			contribution, drawn := o.contributed(id)
			switch {
			case shell:
				card, err = o.card(r, spec, layout.IsClosed(id), csrf)
			case drawn:
				card, err = contributedCard(r, contribution, layout.IsClosed(id), csrf)
			case slices.Contains(o.Kept, id):
				// An installed plugin's card that did not draw keeps its
				// place, so the page's next write of the layout keeps it too.
				card = overviewCardView{ID: id, Kept: true, Closed: layout.IsClosed(id)}
			default:
				continue
			}
			if err != nil {
				return err
			}
			v.Cols[col] = append(v.Cols[col], card)
		}
	}
	for _, id := range layout.Hidden {
		if spec, shell := specs[id]; shell {
			v.Hidden = append(v.Hidden, overviewShelfView{ID: id, Title: r.tr(spec.Title), Tone: spec.head(o, r.tr).Tone})
		} else if c, drawn := o.contributed(id); drawn {
			v.Hidden = append(v.Hidden, overviewShelfView{ID: id, Title: c.Title, Tone: c.Tone})
		} else if slices.Contains(o.Kept, id) {
			v.Hidden = append(v.Hidden, overviewShelfView{ID: id, Kept: true})
		}
	}
	return r.execute(out, "overview.html.tmpl", v)
}

// contributedCard is a plugin's card as the page draws it, its body in the
// card-body pass a shell card's takes, never translated again.
func contributedCard(r *Renderer, c OverviewContribution, closed bool, csrf string) (overviewCardView, error) {
	var body template.HTML
	if c.Body != nil {
		var err error
		if body, err = renderToHTML(r.cardBody(), c.Body, csrf); err != nil {
			return overviewCardView{}, fmt.Errorf("overview card %s: %w", c.ID, err)
		}
	}
	v := overviewCardView{ID: c.ID, Title: c.Title, Icon: c.Icon, Head: c.head(), Closed: closed, Body: body, Href: c.Href}
	if v.Href != "" {
		v.OpenLabel = fmt.Sprintf(r.tr("Open %s"), c.Title)
	}
	return v, nil
}

// card builds one shown card: its header from its head, its body from its
// builder, and its door from the live registry.
func (o *Overview) card(r *Renderer, spec overviewCardSpec, closed bool, csrf string) (overviewCardView, error) {
	body, err := spec.body(o, r.cardBody(), csrf)
	if err != nil {
		return overviewCardView{}, fmt.Errorf("overview card %s: %w", spec.ID, err)
	}
	v := overviewCardView{ID: spec.ID, Title: r.tr(spec.Title), Icon: spec.Icon, Head: spec.head(o, r.tr), Closed: closed, Body: body}
	for _, kind := range spec.doors {
		if door := o.Doors[kind]; door.Href != "" {
			v.Href, v.OpenLabel = door.Href, fmt.Sprintf(r.tr("Open %s"), door.Word)
			break
		}
	}
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
			Name: m.Name, Overview: true, Tone: m.Band, Verbatim: true,
			Label: m.Label, Value: m.Value, Unit: m.Unit, Fill: m.Fill, WarnMark: m.Warn, Detail: m.Detail,
		})
	}
	return out
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html"
	"html/template"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/ports"
)

// Ports renders a device's physical panel — the actual RJ45 and SFP+ connectors, lit
// from their link and traffic — instead of a list of "interfaces" no one recognises.
// It is data-driven: a board without art gets a panel generated from the supplied
// port list, so any device (any count or mix of connectors) renders from data rather
// than hardcoded chrome, and a device may carry an accent for its own identity.
//
// This is the most domain-shaped widget in the set — it names "ports" — but it earns a
// place by ADR-005 §3: the panel is a core, recurring network-appliance visual
// that composition of the generic primitives genuinely cannot express, and it stays
// generic by being driven only by data. The shell owns every pixel (the connector art,
// the palette, the "connected" colour); the caller supplies only the ports and state.
type Ports struct {
	Device string     `json:"device"` // optional device name shown above the panel
	Accent string     `json:"accent"` // identity accent: sky (default) | emerald | violet | amber
	Items  []PortItem `json:"ports"`
	Legend bool       `json:"legend"` // show the explanatory key beneath the panel
	// Back and Front carry a board profile's rear/front panel artwork — trusted,
	// first-party SVG embedded in the shell. When either is set, the shell lights
	// the art's own data-verso-port groups from the live port state and shows it
	// in place of the generated panel; the caller still supplies Items, which
	// drive the lighting. One side shows no side label (which face carries the
	// connectors doesn't matter); both show a Front/Rear control. Empty draws the
	// generated panel. These are server-supplied, never wire-decoded.
	Back  string `json:"-"`
	Front string `json:"-"`
}

// PortItem is one physical connector on the panel.
type PortItem struct {
	Kind  string `json:"kind"`  // "rj45" | "sfp"
	Label string `json:"label"` // plain-language label ("Internet", "Network 1")
	// Verbatim declares the label an identity rather than prose — a board
	// without a profile labels each port with its kernel name (eth0), which
	// the localization walk must leave exactly as authored.
	Verbatim bool   `json:"verbatim,omitempty"`
	Role     string `json:"role"`   // "wan" | "lan" | "" — tints the label for the WAN
	Linked   bool   `json:"linked"` // a cable is connected — the green LED
	// Activity is how busy the amber LED blinks, 0 (dark) to 4 (near solid),
	// from the port's packets per second (ports.Activity). Active is the
	// boolean a plugin may send instead; it reads as level 2.
	Activity int    `json:"activity,omitempty"`
	Active   bool   `json:"active"`
	Speed    string `json:"speed,omitempty"` // negotiated link speed as labelled (1G), a dash without link
	Iface    string `json:"iface"`           // hover detail: interface name
	Addr     string `json:"addr"`            // hover detail: address
	Note     string `json:"note"`            // hover detail: e.g. "PPPoE", "DHCP server"
}

// level is the activity the traffic LED shows: the measured level when there
// is one, else a steady blink for a plugin's boolean active.
func (it PortItem) level() int {
	switch {
	case it.Activity > 0:
		return min(it.Activity, 4)
	case it.Active:
		return 2
	default:
		return 0
	}
}

// phaseText is the fraction of a period the port's blink starts late, as the
// --led-phase custom property carries it.
func phaseText(iface string) string {
	return strconv.FormatFloat(ports.Phase(iface), 'f', -1, 64)
}

func (*Ports) isWidget() {}

func (*Ports) children() []Widget { return nil }

type portsView struct {
	Device    string
	Accent    string
	Items     []PortItem
	Legend    bool
	HasWAN    bool
	HasRJ45   bool
	HasSFP    bool
	PanelBoth bool      // both faces present → show a Front/Rear control
	Back      panelFace // the rear panel, lit from live state
	Front     panelFace // the front panel, lit from live state
	// The link words a stream relabels a port with, in the reader's language.
	WordOn, WordOff string
}

// panelFace is one face of a panel: its lit art and the size it is drawn at
// where it fits, which the stylesheet shrinks from in whole 8px steps.
type panelFace struct {
	Art    template.HTML
	Height float64 // the art's own height in pixels, from its viewBox
	Aspect float64 // height over width
}

// viewBoxSize matches an art's viewBox, whose width and height are its own
// size in pixels.
var viewBoxSize = regexp.MustCompile(`viewBox="0 0 ([0-9.]+) ([0-9.]+)"`)

func newFace(art template.HTML) panelFace {
	face := panelFace{Art: art}
	if m := viewBoxSize.FindStringSubmatch(string(art)); m != nil {
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		if w > 0 && h > 0 {
			face.Height, face.Aspect = h, h/w
		}
	}
	return face
}

func (p *Ports) renderInto(r *Renderer, out io.Writer, _ string) error {
	accent := p.Accent
	switch accent {
	case "emerald", "violet", "amber", "sky":
	default:
		accent = "sky"
	}
	v := portsView{Device: p.Device, Accent: accent, Items: p.Items, Legend: p.Legend,
		WordOn: r.tr("Connected"), WordOff: r.tr("Not connected")}
	// The legend explains only what this device actually has, derived from its ports.
	for _, it := range p.Items {
		switch it.Kind {
		case "rj45":
			v.HasRJ45 = true
		case "sfp":
			v.HasSFP = true
		}
		if it.Role == "wan" {
			v.HasWAN = true
		}
	}
	back, front := p.Back, p.Front
	if back == "" && front == "" && len(p.Items) > 0 {
		// A board without art draws its ports as a generated panel, lit and
		// shown exactly as art is.
		back = generatedPanel(p.Items)
	}
	v.PanelBoth = back != "" && front != ""
	v.Back = newFace(litPanel(back, p.Items, v))
	v.Front = newFace(litPanel(front, p.Items, v))
	return r.execute(out, "ports.html.tmpl", v)
}

// portLabel is what a keyboard or screen reader reaches a port by: its name,
// its link in words, and its speed while it has link.
func portLabel(it PortItem, v portsView) string {
	if !it.Linked {
		return it.Label + ", " + v.WordOff
	}
	label := it.Label + ", " + v.WordOn
	if it.Speed != "" && it.Speed != "—" {
		label += ", " + it.Speed
	}
	return label
}

// litPanel tags a panel's art with the live port state: for each port, the
// shell adds its is-linked / act-N / is-wan classes, its blink phase, and the
// focus and label a keyboard reaches it by onto the art's own data-verso-port
// group, so one set of stylesheet rules lights every panel (ADR-005 — the
// shell owns every pixel). The art is trusted: a profile's is first-party
// content embedded in the binary, and a generated panel escapes what it draws.
func litPanel(svg string, items []PortItem, v portsView) template.HTML {
	if svg == "" {
		return ""
	}
	for _, it := range items {
		if it.Iface == "" {
			continue
		}
		var classes []string
		if it.Linked {
			classes = append(classes, "is-linked")
		}
		classes = append(classes, "act-"+strconv.Itoa(it.level()))
		if it.Role == "wan" {
			classes = append(classes, "is-wan")
		}
		// The panel contract (profiles/README.md, back.svg header) tags each
		// connector group `data-verso-port="<iface>" class="port"`; extend that
		// class list so the lighting rules match, exactly once per port.
		anchor := `data-verso-port="` + html.EscapeString(it.Iface) + `" class="port`
		lit := anchor + " " + strings.Join(classes, " ") + `" style="--led-phase: ` + phaseText(it.Iface) + `"` +
			` tabindex="0" role="img" aria-label="` + html.EscapeString(portLabel(it, v)) + `"`
		svg = strings.Replace(svg, anchor+`"`, lit, 1)
	}
	return template.HTML(svg) //nolint:gosec // first-party art embedded in the binary; only a runtime-contributed profile would need sanitizing, and none exists
}

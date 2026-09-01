// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// Ports renders a device's physical rear panel — the actual RJ45 and SFP+ connectors,
// drawn to scale, lit when a cable is present — instead of a list of "interfaces" no
// one recognises. It is data-driven: the panel is built entirely from the supplied
// port list, so any device (any count or mix of connectors) renders from data rather
// than hardcoded chrome, and a device may carry an accent for its own identity.
//
// This is the most domain-shaped widget in the set — it names "ports" — but it earns a
// place by ADR-005 §5: the rear panel is a core, recurring network-appliance visual
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
	// in place of the generated connector strip; the caller still supplies Items,
	// which drive the lighting. One side shows no side label (which face carries
	// the connectors doesn't matter); both show a Front/Rear control. Empty falls
	// back to the generated strip. These are server-supplied, never wire-decoded.
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
	Active   bool   `json:"active"` // traffic is flowing right now — the amber LED blinks
	Speed    string `json:"speed"`  // link speed shown under the port ("1 Gbps", "—")
	Iface    string `json:"iface"`  // hover detail: interface name
	Addr     string `json:"addr"`   // hover detail: address
	Note     string `json:"note"`   // hover detail: e.g. "PPPoE", "DHCP server"
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
	HasPanel  bool          // render the profile's own panel art, not the generated strip
	PanelBoth bool          // both faces present → show a Front/Rear control
	Back      template.HTML // the rear panel, lit from live state
	Front     template.HTML // the front panel, lit from live state
}

func (p *Ports) renderInto(r *Renderer, out io.Writer, _ string) error {
	accent := p.Accent
	switch accent {
	case "emerald", "violet", "amber", "sky":
	default:
		accent = "sky"
	}
	v := portsView{Device: p.Device, Accent: accent, Items: p.Items, Legend: p.Legend}
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
	if p.Back != "" || p.Front != "" {
		v.HasPanel = true
		v.PanelBoth = p.Back != "" && p.Front != ""
		v.Back = litPanel(p.Back, p.Items)
		v.Front = litPanel(p.Front, p.Items)
	}
	return r.execute(out, "ports.html.tmpl", v)
}

// litPanel tags a profile's panel art with the live port state: for each port,
// the shell adds the same is-linked / is-active / is-wan classes the generated
// strip wears onto the art's own data-verso-port group, so one set of stylesheet
// rules lights both the art and the strip (ADR-005 — the shell owns every pixel).
// The art is trusted first-party content embedded in the binary, rendered as-is.
func litPanel(svg string, items []PortItem) template.HTML {
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
		if it.Active {
			classes = append(classes, "is-active")
		}
		if it.Role == "wan" {
			classes = append(classes, "is-wan")
		}
		if len(classes) == 0 {
			continue
		}
		// The panel contract (profiles/README.md, back.svg header) tags each
		// connector group `data-verso-port="<iface>" class="port"`; extend that
		// class list so the lighting rules match, exactly once per port.
		anchor := `data-verso-port="` + it.Iface + `" class="port`
		svg = strings.Replace(svg, anchor+`"`, anchor+" "+strings.Join(classes, " ")+`"`, 1)
	}
	return template.HTML(svg) //nolint:gosec // first-party art embedded in the binary; only a runtime-contributed profile would need sanitizing, and none exists
}

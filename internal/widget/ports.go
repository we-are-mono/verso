// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

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
}

// PortItem is one physical connector on the panel.
type PortItem struct {
	Kind   string `json:"kind"`   // "rj45" | "sfp"
	Label  string `json:"label"`  // plain-language label ("Internet", "Network 1")
	Role   string `json:"role"`   // "wan" | "lan" | "" — tints the label for the WAN
	Linked bool   `json:"linked"` // a cable is connected — the green LED
	Active bool   `json:"active"` // traffic is flowing right now — the amber LED blinks
	Speed  string `json:"speed"`  // link speed shown under the port ("1 Gbps", "—")
	Iface  string `json:"iface"`  // hover detail: interface name
	Addr   string `json:"addr"`   // hover detail: address
	Note   string `json:"note"`   // hover detail: e.g. "PPPoE", "DHCP server"
}

func (*Ports) isWidget() {}

type portsView struct {
	Device  string
	Accent  string
	Items   []PortItem
	Legend  bool
	HasWAN  bool
	HasRJ45 bool
	HasSFP  bool
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
	return r.execute(out, "ports.html.tmpl", v)
}

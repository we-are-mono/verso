// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
)

// NetMap draws the home network as a left-to-right map: a source (the internet
// link) flows into a hub (this router), which branches to one or more leaves —
// the network's zones (lan, guest, iot, …). It is a shell-owned visual, like
// ports/chart/meter: the plugin declares the nodes, the shell owns the layout,
// the connector geometry, and the animated flow. Live lights the links with a
// green flow that says traffic is moving.
//
// The map is the one place a zone reads as a plain node label, not a chip — here
// the zone IS the node's identity, not metadata hung off a device name.
type NetMap struct {
	Source NetNode   `json:"source"`
	Hub    NetNode   `json:"hub"`
	Leaves []NetNode `json:"leaves"`
	Live   bool      `json:"live,omitempty"`
}

// NetNode is one box on the map. The source and hub show an icon over a label
// with a caption beneath; a leaf reads as a label with its detail on the right.
// Variant "success" tints the box's border and icon (a healthy internet link).
type NetNode struct {
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Variant string `json:"variant,omitempty"` // "" | "success" (the tone vocabulary)
}

func (*NetMap) isWidget() {}

func (*NetMap) children() []Widget { return nil }

// The map's coordinate system: a 1024-wide viewBox the SVG stretches to the
// container (preserveAspectRatio none), so node columns sit at fixed fractions
// while the height stays in pixels. These x's are the connector endpoints — the
// source box's right edge, the hub's left and right edges, the leaf's left edge.
const (
	nmSrcEdgeX  = 143
	nmHubLeftX  = 256
	nmHubRightX = 400
	nmLeafLeftX = 614
)

// Column fractions + widths for the three tiers, and box heights in px. The hub
// sits well left and the leaf column is wide, so a leaf can carry a long zone
// name without truncating.
const (
	nmSrcLeft   = "0"
	nmHubLeft   = "25%"
	nmLeafLeft  = "60%"
	nmNodeWidth = "14%"
	nmLeafWidth = "40%"
	nmNodeH     = 56 // h-14
	nmLeafH     = 48 // h-12
	nmLeafGap   = 28 // vertical gap between stacked leaves
)

type netMapView struct {
	Height int
	Live   bool
	Source netNodeView
	Hub    netNodeView
	Leaves []netNodeView
	Links  []string // SVG path 'd' strings, source→hub then hub→each leaf
}

type netNodeView struct {
	NetNode
	Left  string
	Width string
	Top   int
	Good  bool
}

func (m *NetMap) renderInto(r *Renderer, out io.Writer, _ string) error {
	n := len(m.Leaves)
	// The height wraps the content tightly — the leaf column (or the hub, if it
	// is taller) — so a canvas around the map pads evenly on every side. Leaves
	// stack with a fixed gap and centre as a group against the hub.
	leafGroupH := 0
	if n > 0 {
		leafGroupH = n*nmLeafH + (n-1)*nmLeafGap
	}
	height := leafGroupH
	if height < nmNodeH {
		height = nmNodeH
	}
	midY := height / 2
	groupTop := (height - leafGroupH) / 2

	v := netMapView{Height: height, Live: m.Live}
	v.Source = netNodeView{NetNode: m.Source, Left: nmSrcLeft, Width: nmNodeWidth, Top: midY - nmNodeH/2, Good: m.Source.Variant == "success"}
	v.Hub = netNodeView{NetNode: m.Hub, Left: nmHubLeft, Width: nmNodeWidth, Top: midY - nmNodeH/2, Good: m.Hub.Variant == "success"}
	v.Links = append(v.Links, fmt.Sprintf("M%d,%d H%d", nmSrcEdgeX, midY, nmHubLeftX))
	cp := (nmLeafLeftX - nmHubRightX) * 2 / 5 // control-point reach, scales with the gap
	for i, leaf := range m.Leaves {
		top := groupTop + i*(nmLeafH+nmLeafGap)
		cy := top + nmLeafH/2
		v.Leaves = append(v.Leaves, netNodeView{NetNode: leaf, Left: nmLeafLeft, Width: nmLeafWidth, Top: top, Good: leaf.Variant == "success"})
		// a cubic bezier that leaves the hub level, then eases to the leaf's row
		v.Links = append(v.Links, fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d",
			nmHubRightX, midY, nmHubRightX+cp, midY, nmLeafLeftX-cp, cy, nmLeafLeftX, cy))
	}
	return r.execute(out, "netmap.html.tmpl", v)
}

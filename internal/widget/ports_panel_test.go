// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// svgPort is what a generated panel's port group holds: its direct children
// by element and class, and its texts.
type svgPort struct {
	iface    string
	children []string // "path.verso-port-body", "g.verso-port-mouth", …
	texts    map[string]string
	pins     int
}

// readPanel walks a generated panel and returns its viewBox and its ports.
func readPanel(t *testing.T, svg string) (string, []svgPort) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	var viewBox string
	var ports []svgPort
	depth, portDepth := 0, -1
	inPins := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("generated panel is not well-formed XML: %v\n%s", err, svg)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			attr := map[string]string{}
			for _, a := range el.Attr {
				attr[a.Name.Local] = a.Value
			}
			if el.Name.Local == "svg" {
				viewBox = attr["viewBox"]
			}
			if _, ok := attr["data-verso-port"]; ok && el.Name.Local == "g" {
				ports = append(ports, svgPort{iface: attr["data-verso-port"], texts: map[string]string{}})
				portDepth = depth
				continue
			}
			if portDepth < 0 {
				continue
			}
			p := &ports[len(ports)-1]
			if depth == portDepth+1 {
				p.children = append(p.children, el.Name.Local+"."+attr["class"])
				inPins = attr["class"] == "verso-port-pins"
			}
			if inPins && el.Name.Local == "rect" {
				p.pins++
			}
			if el.Name.Local == "text" {
				var s string
				if err := dec.DecodeElement(&s, &el); err != nil {
					t.Fatal(err)
				}
				p.texts[attr["class"]] = s
				depth--
			}
		case xml.EndElement:
			if depth == portDepth {
				portDepth = -1
			}
			if depth == portDepth+1 {
				inPins = false
			}
			depth--
		}
	}
	return viewBox, ports
}

// TestGeneratedPanelLayout: a board with no art gets a whole panel, 136 tall,
// 24 in on each side, its 48-wide ports 8 apart and 16 where the connector
// kind changes, so every box edge stands on an 8px line at its own size.
func TestGeneratedPanelLayout(t *testing.T) {
	svg := generatedPanel([]PortItem{
		{Kind: "rj45", Label: "eth0", Iface: "eth0", Speed: "1G"},
		{Kind: "rj45", Label: "eth1", Iface: "eth1", Speed: "—"},
		{Kind: "sfp", Label: "eth2", Iface: "eth2", Speed: "10G"},
	})
	viewBox, ports := readPanel(t, svg)
	// 24 + 48 + 8 + 48 + 16 + 48 + 24
	if viewBox != "0 0 216 136" {
		t.Errorf("viewBox = %q, want 0 0 216 136", viewBox)
	}
	// Each port's outline starts 3.5 in from its box's left edge (the corner
	// radius, its stroke half a pixel in): boxes at 24, 80 and 144. An RJ45's
	// body starts at 24, under the line its WAN tag stands on, an SFP+
	// cage's at 32.
	for _, want := range []string{`d="M27.5,24.5 `, `d="M83.5,24.5 `, `d="M147.5,32.5 `} {
		if !strings.Contains(svg, want) {
			t.Errorf("generated panel missing outline %q:\n%s", want, svg)
		}
	}
	if len(ports) != 3 {
		t.Fatalf("ports = %d, want 3", len(ports))
	}
}

// TestGeneratedPanelPorts: every port is a group the shell lights like board
// art's: its outline the group's only direct-child path (what the WAN mark
// strokes), one link and one traffic LED, an RJ45 its lip-following mouth
// and eight pins, an SFP+ cage its mouth and socket, and its name and speed
// under it.
func TestGeneratedPanelPorts(t *testing.T) {
	_, ports := readPanel(t, generatedPanel([]PortItem{
		{Kind: "rj45", Label: "lan1", Iface: "lan1", Speed: "1G"},
		{Kind: "sfp", Label: "wan", Iface: "wan", Speed: "—"},
	}))
	for _, p := range ports {
		paths := 0
		for _, c := range p.children {
			if strings.HasPrefix(c, "path.") {
				paths++
			}
		}
		if paths != 1 || p.children[0] != "path.verso-port-body" {
			t.Errorf("%s: direct children %v, want the outline as the only path, first", p.iface, p.children)
		}
		for _, want := range []string{"rect.led-link", "rect.led-traffic"} {
			if !containsString(p.children, want) {
				t.Errorf("%s: missing %s in %v", p.iface, want, p.children)
			}
		}
	}
	rj, sfp := ports[0], ports[1]
	if !containsString(rj.children, "g.verso-port-mouth") || rj.pins != 8 {
		t.Errorf("RJ45: children %v with %d pins, want its mouth group and 8 pins", rj.children, rj.pins)
	}
	if !containsString(sfp.children, "rect.verso-port-mouth") || !containsString(sfp.children, "rect.verso-port-socket") || sfp.pins != 0 {
		t.Errorf("SFP+: children %v, want a mouth and a socket and no pins", sfp.children)
	}
	if rj.texts["verso-port-label"] != "lan1" || rj.texts["verso-port-speed"] != "1G" {
		t.Errorf("RJ45 texts = %v, want lan1 and 1G", rj.texts)
	}
	// Each port carries an empty role slot over it, which the shell fills on
	// the WAN port.
	for _, p := range ports {
		if role, ok := p.texts["verso-port-role"]; !ok || role != "" {
			t.Errorf("%s: role slot = %q (present %v), want an empty slot", p.iface, role, ok)
		}
	}
	if sfp.texts["verso-port-speed"] != "—" {
		t.Errorf("SFP+ speed = %q, want a dash", sfp.texts["verso-port-speed"])
	}
}

// TestGeneratedPanelIsThemable: the generated panel paints nothing itself, so
// the stylesheet colours it for light and dark, and a name is text, never
// markup.
func TestGeneratedPanelIsThemable(t *testing.T) {
	svg := generatedPanel([]PortItem{{Kind: "rj45", Label: `<b>x</b>`, Iface: `a"b`}})
	if strings.Contains(svg, "fill=") || strings.Contains(svg, "stroke=") {
		t.Errorf("generated panel should colour by class, not attribute:\n%s", svg)
	}
	if strings.Contains(svg, "<b>") || !strings.Contains(svg, "&lt;b&gt;x&lt;/b&gt;") || !strings.Contains(svg, `data-verso-port="a&#34;b"`) {
		t.Errorf("names must be escaped:\n%s", svg)
	}
	if !strings.Contains(svg, "data-verso-port-speed") {
		t.Errorf("a live port carries its speed slot:\n%s", svg)
	}
}

// TestPortsWithoutArtDrawTheGeneratedPanel: a board with no art shows the
// generated panel, lit by the same pass as board art.
func TestPortsWithoutArtDrawTheGeneratedPanel(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "eth0", Iface: "eth0", Linked: true, Activity: 3}}})
	for _, want := range []string{"verso-panel-art", "verso-panel-chassis", `data-verso-port="eth0" class="port is-linked act-3"`} {
		if !strings.Contains(got, want) {
			t.Errorf("generated panel missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verso-ports-chassis") {
		t.Errorf("the generated strip is gone:\n%s", got)
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

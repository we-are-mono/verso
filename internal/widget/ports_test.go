// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRenderPorts(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Device: "Mono Gateway", Accent: "emerald", Items: []PortItem{
		{Kind: "rj45", Label: "Network 1", Role: "lan", Linked: true, Speed: "1 Gbps", Iface: "eth0", Addr: "192.168.1.1"},
		{Kind: "sfp", Label: "Internet", Role: "wan", Linked: false, Speed: "—", Iface: "eth4", Note: "PPPoE"},
	}})
	for _, want := range []string{
		"Mono Gateway", "verso-ports--emerald",
		"Network 1", "Internet",
		"verso-port-jack", "verso-port-cage", // both connector kinds drawn
		"is-linked", "is-empty", "is-wan",
		"eth0", "PPPoE", // hover detail
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ports missing %q:\n%s", want, got)
		}
	}
}

// TestPortsLegend: the legend renders only when asked, and lists only the connector
// kinds/roles the device actually has (derived from the port list).
func TestPortsLegend(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Legend: true, Items: []PortItem{
		{Kind: "rj45", Label: "a"}, {Kind: "sfp", Label: "b", Role: "wan"},
	}})
	// Legend text now flows through the {{ t }} localization function (ADR-012),
	// so html/template autoescaping encodes the "+" as &#43; in the source — the
	// browser still renders "SFP+ = fiber".
	for _, want := range []string{"verso-ports-legend", "Connected", "Not connected", "Internet = your WAN", "RJ45 = network cable", "SFP&#43; = fiber"} {
		if !strings.Contains(got, want) {
			t.Errorf("legend missing %q:\n%s", want, got)
		}
	}
	// A copper-only device with no WAN omits the fiber and Internet hints.
	rj := render(t, r, &Ports{Legend: true, Items: []PortItem{{Kind: "rj45", Label: "a", Role: "lan"}}})
	if strings.Contains(rj, "SFP&#43; = fiber") || strings.Contains(rj, "Internet = your WAN") {
		t.Errorf("legend should list only what the device has:\n%s", rj)
	}
	// No legend unless requested.
	if strings.Contains(render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "a"}}}), "verso-ports-legend") {
		t.Error("legend should be absent when Legend is false")
	}
}

// TestPortsAccentDefault: an unknown/absent accent falls back to sky.
func TestPortsAccentDefault(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "x"}}})
	if !strings.Contains(got, "verso-ports--sky") {
		t.Errorf("default accent should be sky:\n%s", got)
	}
}

// TestDecodePorts covers the wire path (Decode → render) including the port list.
func TestDecodePorts(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"ports","device":"D","accent":"violet","ports":[
		{"kind":"sfp","label":"Internet","role":"wan","linked":true,"speed":"10 Gbps","iface":"eth4","note":"PPPoE"}
	]}`))
	if err != nil {
		t.Fatalf("decode ports: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"D", "verso-ports--violet", "Internet", "verso-port-cage", "is-linked", "is-wan", "PPPoE"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded ports missing %q:\n%s", want, got)
		}
	}
}

// TestPortsLiveHooks: a port with an Iface carries the handle and the tip's
// addr/link slots the overview stream updates in place — rendered even while
// empty, so a later reading has a place to land.
func TestPortsLiveHooks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "Internet", Role: "wan", Iface: "wan0"}}})
	for _, want := range []string{`data-verso-port="wan0"`, "data-verso-port-addr", "data-verso-port-link", "verso-port-speed"} {
		if !strings.Contains(got, want) {
			t.Errorf("live port missing %q:\n%s", want, got)
		}
	}
}

// TestPortsWithoutIfaceIsStatic: no Iface, no live handle, and empty detail
// slots stay unrendered.
func TestPortsWithoutIfaceIsStatic(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "x"}}})
	if strings.Contains(got, "data-verso-port") {
		t.Errorf("static port must not carry live hooks:\n%s", got)
	}
}

// TestPortsPanelArt: a profile's own back-panel SVG renders in place of the
// generated strip, lit from the live port state — the shell tags the art's own
// data-verso-port groups with the same is-linked/is-active/is-wan classes the
// strip wears, so one stylesheet lights both.
func TestPortsPanelArt(t *testing.T) {
	r := newRenderer(t)
	back := `<svg viewBox="0 0 100 40"><g data-verso-port="eth0" class="port"><rect class="led-link"/></g><g data-verso-port="eth3" class="port"><path/></g></svg>`
	got := render(t, r, &Ports{
		Back: back,
		Items: []PortItem{
			{Iface: "eth0", Linked: true, Active: true},
			{Iface: "eth3", Role: "wan", Linked: true},
		},
	})
	for _, want := range []string{
		"verso-panel-art",
		`data-verso-port="eth0" class="port is-linked is-active"`, // link + traffic tagged
		`data-verso-port="eth3" class="port is-linked is-wan"`,    // link + WAN role tagged
		"led-link", // the art's own live layer survives intact
	} {
		if !strings.Contains(got, want) {
			t.Errorf("panel art missing %q:\n%s", want, got)
		}
	}
	// The generated chassis strip is not drawn when panel art is present.
	if strings.Contains(got, "verso-ports-chassis") {
		t.Errorf("panel art should replace the generated strip:\n%s", got)
	}
}

// TestPortsGeneratedStripFallback: with no panel art, the generated connector
// strip is still drawn.
func TestPortsGeneratedStripFallback(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "eth0", Iface: "eth0", Linked: true}}})
	if !strings.Contains(got, "verso-ports-chassis") {
		t.Errorf("a board with no panel art falls back to the strip:\n%s", got)
	}
	if strings.Contains(got, "verso-panel-art") {
		t.Errorf("no panel art means no panel container:\n%s", got)
	}
}

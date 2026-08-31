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

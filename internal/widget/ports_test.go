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
		{Kind: "rj45", Label: "Network 1", Role: "lan", Linked: true, Iface: "eth0", Addr: "192.168.1.1"},
		{Kind: "sfp", Label: "Internet", Role: "wan", Linked: false, Iface: "eth4", Note: "PPPoE"},
	}})
	for _, want := range []string{
		`<div class="verso-ports__name">Mono Gateway</div>`, `<div class="verso-ports" data-verso-accent="emerald"`,
		"Network 1", "Internet",
		"verso-panel__pins", "verso-panel__socket", // both connector kinds drawn
		`data-verso-port="eth0" class="port" data-verso-activity="0" data-verso-linked style=`,
		`data-verso-port="eth4" class="port" data-verso-activity="0" data-verso-wan style=`,
		`<div class="verso-ports__tip" data-verso-port-tip="eth0"`, "192.168.1.1", "PPPoE", // hover detail
		`<b class="verso-ports__tip-name">Internet</b>`, `<span class="verso-ports__tip-role">wan</span>`,
		`<div class="verso-ports__tip-row" data-verso-verbatim><span class="verso-ports__tip-key">iface</span>eth0</div>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ports missing %q:\n%s", want, got)
		}
	}
	// The device's accent tints its name.
	declares(t, "ports", `.verso-ports[data-verso-accent="emerald"] .verso-ports__name`, "color: var(--color-green-line);")
	declares(t, "ports", ".verso-ports__name", "@apply type-label;")
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
	for _, want := range []string{
		`<div class="verso-ports__legend">`,
		`<span class="verso-ports__key"><span class="verso-ports__dot" data-verso-state="on"></span>Connected</span>`,
		`<span class="verso-ports__key"><span class="verso-ports__dot" data-verso-state="act"></span>Traffic</span>`,
		`<span class="verso-ports__key"><span class="verso-ports__dot"></span>Not connected</span>`,
		"Internet = your WAN", "RJ45 = network cable", "SFP&#43; = fiber",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("legend missing %q:\n%s", want, got)
		}
	}
	// Each dot is lit as the LED it stands for.
	declares(t, "ports", `.verso-ports__dot[data-verso-state="on"]`, "background: var(--color-green);")
	declares(t, "ports", `.verso-ports__dot[data-verso-state="act"]`, "background: var(--color-marigold);")
	declares(t, "ports", ".verso-ports__dot", "background: var(--color-inert);")
	// A copper-only device with no WAN omits the fiber and Internet hints.
	rj := render(t, r, &Ports{Legend: true, Items: []PortItem{{Kind: "rj45", Label: "a", Role: "lan"}}})
	if strings.Contains(rj, "SFP&#43; = fiber") || strings.Contains(rj, "Internet = your WAN") {
		t.Errorf("legend should list only what the device has:\n%s", rj)
	}
	// No legend unless requested.
	if strings.Contains(render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "a"}}}), "verso-ports__legend") {
		t.Error("legend should be absent when Legend is false")
	}
}

// TestPortsAccentDefault: an unknown/absent accent falls back to sky.
func TestPortsAccentDefault(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "x"}}})
	if !strings.Contains(got, `<div class="verso-ports" data-verso-accent="sky"`) {
		t.Errorf("default accent should be sky:\n%s", got)
	}
	declares(t, "ports", `.verso-ports[data-verso-accent="sky"] .verso-ports__name`, "color: var(--color-denim-line);")
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
	for _, want := range []string{
		"D", `<div class="verso-ports" data-verso-accent="violet"`, "Internet", "verso-panel__socket",
		`data-verso-port="eth4" class="port" data-verso-activity="0" data-verso-linked data-verso-wan style=`,
		"PPPoE", ">10 Gbps</text>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded ports missing %q:\n%s", want, got)
		}
	}
}

// TestPortsLiveHooks: a port with an Iface carries the handle, its speed slot
// and its detail's addr/link slots the overview stream updates in place —
// rendered even while empty, so a later reading has a place to land.
func TestPortsLiveHooks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Items: []PortItem{{Kind: "rj45", Label: "Internet", Role: "wan", Iface: "wan0"}}})
	for _, want := range []string{`data-verso-port="wan0"`, `data-verso-port-tip="wan0"`, "data-verso-port-addr", "data-verso-port-link", "data-verso-port-speed"} {
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
// data-verso-port groups with the same data-verso-linked/-activity/-wan state
// the generated panel wears, so one stylesheet lights both.
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
		`<div class="verso-panel"`,
		`data-verso-port="eth0" class="port" data-verso-activity="2" data-verso-linked style=`,                // link + traffic tagged
		`data-verso-port="eth3" class="port" data-verso-activity="0" data-verso-linked data-verso-wan style=`, // link + WAN role tagged
		"led-link", // the art's own live layer survives intact
	} {
		if !strings.Contains(got, want) {
			t.Errorf("panel art missing %q:\n%s", want, got)
		}
	}
	// The panel block lights what litPanel tagged: the link LED of a linked
	// port and the WAN port's outline.
	declares(t, "panel", ".verso-panel [data-verso-port][data-verso-linked] .led-link", "fill: var(--color-hw-led-link);")
	declares(t, "panel", ".verso-panel [data-verso-port][data-verso-wan] > path", "stroke: var(--color-hw-wan);", "stroke-width: 1.5;")
	// The generated chassis strip is not drawn when panel art is present.
	if strings.Contains(got, "verso-ports-chassis") {
		t.Errorf("panel art should replace the generated strip:\n%s", got)
	}
}

// TestPortsActivityLevel: a port's traffic LED shows the level the shell
// measured; a plugin that sends only the boolean active keeps a steady blink,
// and a level wins when both arrive. Each lit port also carries the phase its
// blink starts at, the same on every render.
func TestPortsActivityLevel(t *testing.T) {
	r := newRenderer(t)
	back := `<svg viewBox="0 0 100 40"><g data-verso-port="a" class="port"></g><g data-verso-port="b" class="port"></g><g data-verso-port="c" class="port"></g></svg>`
	got := render(t, r, &Ports{Back: back, Items: []PortItem{
		{Iface: "a", Activity: 3, Active: true}, {Iface: "b", Active: true}, {Iface: "c"},
	}})
	for _, want := range []string{
		`data-verso-port="a" class="port" data-verso-activity="3" style="--led-phase: ` + phaseText("a") + `" tabindex="0"`,
		`data-verso-port="b" class="port" data-verso-activity="2" style=`,
		`data-verso-port="c" class="port" data-verso-activity="0" style=`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("activity missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "is-active") {
		t.Errorf("the boolean class is gone; the level carries traffic:\n%s", got)
	}
	// Each level is its own beat, and level 0 leaves the LED dark.
	for level, beat := range map[string][]string{
		"1": {"--led-beat: verso-led-blip;", "--led-period: 1.8s;"},
		"2": {"--led-beat: verso-led-blink;", "--led-period: 0.5s;"},
		"3": {"--led-beat: verso-led-blink;", "--led-period: 0.16s;"},
		"4": {"--led-beat: verso-led-flicker;", "--led-period: 0.08s;"},
	} {
		declares(t, "panel", `.verso-panel [data-verso-activity="`+level+`"]`, beat...)
	}
	declares(t, "panel", `.verso-panel [data-verso-port][data-verso-activity]:not([data-verso-activity="0"]) .led-traffic`,
		"fill: var(--color-hw-led-act);", "animation: var(--led-beat) var(--led-period) steps(1, end) infinite;")

	w, err := Decode([]byte(`{"type":"ports","ports":[{"kind":"rj45","label":"x","iface":"a","active":true}]}`))
	if err != nil {
		t.Fatalf("decode ports: %v", err)
	}
	ports := w.(*Ports)
	ports.Back = back
	if got := render(t, r, ports); !strings.Contains(got, `data-verso-port="a" class="port" data-verso-activity="2" style=`) {
		t.Errorf("a plugin's boolean active should read as a steady blink:\n%s", got)
	}
}

// TestTheWANPortIsTaggedAboveIt: the art carries an empty role slot over each
// port, never the role itself; the shell writes "WAN" into the WAN port's slot
// alone, a step heavier and spaced as part of the drawing, and the port's
// label names the role too.
func TestTheWANPortIsTaggedAboveIt(t *testing.T) {
	r := newRenderer(t)
	back := `<svg viewBox="0 0 100 40">` +
		`<g data-verso-port="eth0" class="port"><path/><text class="verso-port-role" x="10" y="8"></text><text>eth0</text></g>` +
		`<g data-verso-port="eth1" class="port"><path/><text class="verso-port-role" x="60" y="8"></text><text>eth1</text></g></svg>`
	got := render(t, r, &Ports{Back: back, Items: []PortItem{
		{Label: "eth0", Iface: "eth0", Role: "wan", Linked: true, Speed: "1G"}, {Label: "eth1", Iface: "eth1"},
	}})
	for _, want := range []string{
		`<text class="verso-port-role" x="10" y="8" font-weight="600" letter-spacing="0.04em">WAN</text>`,
		`<text class="verso-port-role" x="60" y="8"></text>`,
		`aria-label="eth0, WAN, Connected, 1G"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("WAN tag missing %q:\n%s", want, got)
		}
	}
	// The word is the hardware blue on board art's dark finish.
	declares(t, "panel", ".verso-panel .verso-port-role", "fill: var(--color-hw-wan);")
}

// TestPortsArtSizesFromItsViewBox: a panel's own size reaches the stylesheet,
// which draws it at that size where it fits and shrinks it in whole 8px steps
// where it does not.
func TestPortsArtSizesFromItsViewBox(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Ports{Back: `<svg viewBox="0 0 504 136"></svg>`, Items: []PortItem{{Iface: "eth0"}}})
	if !strings.Contains(got, `<div class="verso-panel__face" style="--art-h: 136px; --art-aspect: 0.2698412698412698">`) {
		t.Errorf("panel face should carry its native height and aspect:\n%s", got)
	}
	declares(t, "panel", ".verso-panel__face svg", "height: min(var(--art-h), round(down, 100cqi * var(--art-aspect), 0.5rem));")
	// The panel says how wide its widest face is drawn, so a column that
	// holds it can be exactly as wide.
	got = render(t, r, &Ports{Back: `<svg viewBox="0 0 504 136"></svg>`, Front: `<svg viewBox="0 0 432 136"></svg>`, Items: []PortItem{{Iface: "eth0"}}})
	if !strings.Contains(got, `<div class="verso-ports" data-verso-accent="sky" style="--art-w: 504px">`) {
		t.Errorf("the panel should carry its widest face's width:\n%s", got)
	}
}

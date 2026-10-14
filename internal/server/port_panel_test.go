// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"slices"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// fakeKinds answers each port's connector; anything unnamed is copper.
type fakeKinds map[string]string

func (f fakeKinds) Kind(iface string) string {
	if kind, ok := f[iface]; ok {
		return kind
	}
	return "rj45"
}

// physical is a port as the sampler reports it, with its last second's
// packets and its negotiated speed.
func physical(name, state string, pps uint64, speed int) telemetry.Interface {
	return telemetry.Interface{
		Name: name, Kind: "port", Physical: true, Operstate: state, SpeedMbps: speed,
		History: []telemetry.Point{{RxPPS: pps / 2, TxPPS: pps - pps/2}},
	}
}

func panelNames(p *widget.Ports) []string {
	var names []string
	for _, it := range p.Items {
		names = append(names, it.Iface)
	}
	return names
}

// TestDevicePanelOnTheDK: the DK's profile orders its ports and brings its
// own art; each port carries its live link, activity and speed.
func TestDevicePanelOnTheDK(t *testing.T) {
	srv := newServer(t, fakeBackend{wan: openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "eth0"}}}})
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{Interfaces: []telemetry.Interface{
		physical("eth0", "up", 50000, 1000), physical("eth1", "down", 0, -1), physical("eth2", "up", 3, 1000),
		physical("eth3", "up", 0, 10000), physical("eth4", "down", 0, -1),
	}}}
	srv.portKinds = fakeKinds{"eth3": "sfp", "eth4": "sfp"}
	profile, ok := sensors.LoadProfile(profiles.FS, "mono,gateway-dk")
	if !ok {
		t.Fatal("the DK's profile should load")
	}
	panel := srv.devicePanel(context.Background(), "sid", profile)
	if panel == nil || panel.Back == "" {
		t.Fatalf("the DK's panel should carry its art: %+v", panel)
	}
	if got, want := panelNames(panel), []string{"eth1", "eth2", "eth0", "eth3", "eth4"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want the profile's %v", got, want)
	}
	byName := map[string]widget.PortItem{}
	for _, it := range panel.Items {
		byName[it.Iface] = it
	}
	for name, want := range map[string]widget.PortItem{
		"eth0": {Kind: "rj45", Linked: true, Activity: 3, Speed: "1G", Role: "wan"},
		"eth1": {Kind: "rj45", Linked: false, Activity: 0, Speed: "—"},
		"eth2": {Kind: "rj45", Linked: true, Activity: 1, Speed: "1G"},
		"eth3": {Kind: "sfp", Linked: true, Activity: 0, Speed: "10G"},
	} {
		got := byName[name]
		if got.Kind != want.Kind || got.Linked != want.Linked || got.Activity != want.Activity || got.Speed != want.Speed || got.Role != want.Role {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
}

// TestDevicePanelWithoutAProfile: a board no profile describes draws its
// ports in its board description's order, the rest after, each as the
// connector its driver reports; a DSA conduit is no port, and the port a WAN
// VLAN rides on carries the WAN mark.
func TestDevicePanelWithoutAProfile(t *testing.T) {
	srv := newServer(t, fakeBackend{wan: openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "wan.7"}}}})
	conduit := physical("eth0", "up", 9000, 1000)
	conduit.Conduit = true
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{Interfaces: []telemetry.Interface{
		conduit, physical("lan2", "down", 0, -1), physical("lan1", "up", 20, 1000), physical("wan", "up", 0, 2500),
		physical("usb0", "up", 0, 100),
		{Name: "wan.7", Kind: "vlan", Parent: "wan"}, {Name: "br-lan", Kind: "bridge"},
	}}}
	srv.portKinds = fakeKinds{"wan": "sfp"}
	srv.boardJSON = func() []byte {
		return []byte(`{"network":{"lan":{"ports":["lan1","lan2"]},"wan":{"device":"wan"}}}`)
	}
	panel := srv.devicePanel(context.Background(), "sid", nil)
	if panel == nil {
		t.Fatal("a board with ports gets a panel")
	}
	if panel.Back != "" || panel.Front != "" {
		t.Errorf("a board without a profile has no art of its own")
	}
	if got, want := panelNames(panel), []string{"wan", "lan1", "lan2", "usb0"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if wan := panel.Items[0]; wan.Kind != "sfp" || wan.Role != "wan" || wan.Speed != "2.5G" || wan.Label != "wan" || !wan.Verbatim {
		t.Errorf("wan = %+v, want an SFP+ port marked WAN, labelled by its name, at 2.5G", wan)
	}
}

// TestDevicePanelWithoutPorts: a box with no physical port shows no panel.
func TestDevicePanelWithoutPorts(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{Interfaces: []telemetry.Interface{{Name: "lo", Kind: "loopback"}}}}
	if panel := srv.devicePanel(context.Background(), "sid", nil); panel != nil {
		t.Errorf("no ports, no panel: %+v", panel)
	}
}

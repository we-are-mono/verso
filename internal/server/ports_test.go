// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// portsBackend declares the testbed's shape in uci: a wan interface on wan0
// (DHCP), a lan interface on the br-lan bridge with one member port.
func portsBackend() fakeBackend {
	return fakeBackend{uci: map[string]map[string]any{"network": {
		"loopback": map[string]any{".type": "interface", ".name": "loopback", "device": "lo"},
		"wan":      map[string]any{".type": "interface", ".name": "wan", "device": "wan0", "proto": "dhcp"},
		"lan":      map[string]any{".type": "interface", ".name": "lan", "device": "br-lan", "ipaddr": "192.168.77.1"},
		"cfg0f15": map[string]any{
			".type": "device", ".name": "cfg0f15",
			"name": "br-lan", "type": "bridge", "ports": []any{"lan0"},
		},
	}}}
}

// TestPortList: the panel comes from config (lan bridge members as Network N,
// the uplink as Internet last) and lights from live device state; the wan
// port carries the uplink's address and protocol.
func TestPortList(t *testing.T) {
	be := portsBackend()
	be.wan = openwrt.WANState{Up: true, Device: "wan0", Addr: "172.30.1.178"}
	stats := []openwrt.DeviceStats{
		{Carrier: true, SpeedMbps: 10000}, // lan0
		{Carrier: true, SpeedMbps: 1000},  // wan0
	}
	be.devStats = &stats

	items, counters := newServer(t, be).portList(context.Background(), "test-sid")
	if len(items) != 2 {
		t.Fatalf("got %d ports, want 2: %+v", len(items), items)
	}
	if counters["lan0"] != 0 || counters["wan0"] != 0 {
		t.Errorf("counters = %+v, want zeroed totals", counters)
	}
	lan := items[0]
	if lan.Kind != "rj45" || lan.Label != "Network 1" || lan.Role != "lan" || !lan.Linked ||
		lan.Speed != "10 Gbps" || lan.Iface != "lan0" || lan.Addr != "192.168.77.1" {
		t.Errorf("lan port = %+v", lan)
	}
	wan := items[1]
	if wan.Label != "Internet" || wan.Role != "wan" || !wan.Linked || wan.Speed != "1 Gbps" ||
		wan.Iface != "wan0" || wan.Addr != "172.30.1.178" || wan.Note != "DHCP" {
		t.Errorf("wan port = %+v", wan)
	}
}

// TestPortListDegrades: no readable network config means no panel; unreadable
// device stats keep the connector, unlit — the port exists even when its
// state cannot be read.
func TestPortListDegrades(t *testing.T) {
	if items, _ := newServer(t, fakeBackend{}).portList(context.Background(), "test-sid"); len(items) != 0 {
		t.Fatalf("no config: got %+v, want none", items)
	}

	be := portsBackend()
	be.devErr = errors.New("no such device")
	items, _ := newServer(t, be).portList(context.Background(), "test-sid")
	if len(items) != 2 {
		t.Fatalf("stats down: got %d ports, want 2", len(items))
	}
	if items[0].Linked || items[0].Speed != "—" {
		t.Errorf("stats down: lan port = %+v, want unlit with a dash", items[0])
	}
}

// TestMarkPortActivity: the amber LED follows the byte delta between two
// samples — past the floor lights it, murmur below the floor does not, and
// the very first sample (no previous) marks nothing.
func TestMarkPortActivity(t *testing.T) {
	items := []widget.PortItem{{Iface: "lan0"}, {Iface: "wan0"}}
	markPortActivity(items, map[string]int64{"lan0": 9000, "wan0": 100}, nil)
	if items[0].Active || items[1].Active {
		t.Fatalf("first sample must mark nothing: %+v", items)
	}
	markPortActivity(items,
		map[string]int64{"lan0": 9000, "wan0": 100},
		map[string]int64{"lan0": 1000, "wan0": 90})
	if !items[0].Active {
		t.Error("lan0 moved 8000 bytes: want active")
	}
	if items[1].Active {
		t.Error("wan0 moved 10 bytes: want idle")
	}
}

// TestParseNetworkTopologyPlainDevice: a lan interface sitting directly on a
// device (no bridge section) contributes that device as the one port.
func TestParseNetworkTopologyPlainDevice(t *testing.T) {
	topo := parseNetworkTopology(map[string]any{
		"lan": map[string]any{".type": "interface", ".name": "lan", "device": "eth1", "ipaddr": "10.0.0.1"},
	})
	if len(topo.lanPorts) != 1 || topo.lanPorts[0] != "eth1" || topo.lanAddr != "10.0.0.1" {
		t.Fatalf("topology = %+v", topo)
	}
}

// TestLinkSpeed: negotiated links read the way a person says them; no carrier
// or no reported speed is a quiet dash.
func TestLinkSpeed(t *testing.T) {
	cases := map[string]openwrt.DeviceStats{
		"—":        {Carrier: false, SpeedMbps: 1000},
		"10 Gbps":  {Carrier: true, SpeedMbps: 10000},
		"1 Gbps":   {Carrier: true, SpeedMbps: 1000},
		"100 Mbps": {Carrier: true, SpeedMbps: 100},
	}
	for want, st := range cases {
		if got := linkSpeed(st); got != want {
			t.Errorf("linkSpeed(%+v) = %q, want %q", st, got, want)
		}
	}
}

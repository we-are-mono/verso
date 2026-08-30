// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
)

func TestParseVLANPort(t *testing.T) {
	cases := []struct {
		spec       string
		wantName   string
		wantTagged bool
	}{
		{"lan1", "lan1", false},
		{"lan2:t", "lan2", true},
		{"lan1:u*", "lan1", false}, // untagged pvid
		{"lan3:t*", "lan3", true},
	}
	for _, c := range cases {
		name, tagged := parseVLANPort(c.spec)
		if name != c.wantName || tagged != c.wantTagged {
			t.Errorf("parseVLANPort(%q) = (%q, %v), want (%q, %v)", c.spec, name, tagged, c.wantName, c.wantTagged)
		}
	}
}

func TestDeviceVLAN(t *testing.T) {
	cases := []struct{ device, bridge, id string }{
		{"br-lan.20", "br-lan", "20"},
		{"br-lan", "br-lan", ""},
		{"eth0.5", "eth0", "5"},
		{"eth0", "eth0", ""},
		{"br-lan.name", "br-lan.name", ""}, // non-numeric suffix is not a VLAN
	}
	for _, c := range cases {
		b, id := deviceVLAN(c.device)
		if b != c.bridge || id != c.id {
			t.Errorf("deviceVLAN(%q) = (%q, %q), want (%q, %q)", c.device, b, id, c.bridge, c.id)
		}
	}
}

func TestPortVLANsAndParse(t *testing.T) {
	// One bridge-vlan section: VLAN 20 tagged on lan1, untagged on lan2.
	cfg := map[string]any{
		"cfg1": map[string]any{
			".type": "bridge-vlan", "device": "br-lan", "vlan": "20",
			"ports": []any{"lan1:t", "lan2"},
		},
		"cfg2": map[string]any{
			".type": "bridge-vlan", "device": "br-lan", "vlan": "10",
			"ports": []any{"lan1:t"},
		},
	}
	vlans := parseBridgeVLANs(cfg)
	if len(vlans) != 2 {
		t.Fatalf("parseBridgeVLANs: got %d sections", len(vlans))
	}

	byPort := portVLANs(vlans)
	if byPort["lan1"] != "10, 20" { // both VLANs, numerically ordered
		t.Errorf("lan1 VLANs = %q, want %q", byPort["lan1"], "10, 20")
	}
	if byPort["lan2"] != "20" {
		t.Errorf("lan2 VLANs = %q, want %q", byPort["lan2"], "20")
	}
}

func TestInterfaceListUsesTelemetryAsItsRowSource(t *testing.T) {
	be := fakeBackend{
		uci: map[string]map[string]any{
			"network": {
				"lan": map[string]any{
					".type": "interface", ".name": "lan", "device": "br-lan.10",
					"proto": "static", "ipaddr": "192.168.10.1", "netmask": "255.255.255.0",
				},
				"wan": map[string]any{
					".type": "interface", ".name": "wan", "device": "eth4.3900", "proto": "pppoe",
				},
			},
			"firewall": {
				"lan_zone": map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}},
				"wan_zone": map[string]any{".type": "zone", "name": "wan", "network": []any{"wan"}},
			},
		},
		wan: openwrt.WANState{Device: "pppoe-wan"},
	}
	snapshot := telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth0", Kind: "port", Physical: true, Operstate: "down", History: []telemetry.Point{{}}},
		{Name: "eth1", Kind: "port", Physical: true, Operstate: "down", History: []telemetry.Point{{}}},
		{Name: "eth2", Kind: "port", Physical: true, Operstate: "up", History: []telemetry.Point{{TxBPS: 13_955_600}}},
		{Name: "eth3", Kind: "port", Physical: true, Operstate: "up", History: []telemetry.Point{{RxBPS: 258_672}}},
		{Name: "eth4", Kind: "port", Physical: true, Operstate: "up", Members: nil, History: []telemetry.Point{{RxBPS: 13_867_160}}},
		{Name: "br-lan", Kind: "bridge", Operstate: "up", Members: []string{"eth2", "eth3"}, History: []telemetry.Point{{}}},
		{Name: "br-lan.10", Kind: "vlan", Operstate: "up", Parent: "br-lan", History: []telemetry.Point{{RxBytes: 7 << 30}}},
		{Name: "eth4.3900", Kind: "vlan", Operstate: "up", Parent: "eth4", History: []telemetry.Point{{}}},
		{Name: "pppoe-wan", Kind: "pppoe", Operstate: "unknown", History: []telemetry.Point{{}}},
		{Name: "tailscale0", Kind: "tunnel", Operstate: "unknown", History: []telemetry.Point{{}}},
	}}

	rows := newServer(t, be).interfaceList(context.Background(), "test-sid", snapshot)
	if len(rows) != len(snapshot.Interfaces) {
		t.Fatalf("rows = %d, want every one of %d telemetry interfaces", len(rows), len(snapshot.Interfaces))
	}
	for index, name := range []string{"eth0", "eth1", "eth2", "eth3", "eth4"} {
		if rows[index].Name != name || !rows[index].Physical || rows[index].Kind != "port" {
			t.Fatalf("physical row %d = %+v", index, rows[index])
		}
	}
	byName := map[string]widget.OverviewInterface{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	if row := byName["br-lan.10"]; len(row.Networks) != 1 || row.Networks[0] != "lan" ||
		row.Zone != "lan" || row.Subnet != "192.168.10.0/24" || row.VLAN != "10" || row.RxTotal != "7.0 GiB" {
		t.Errorf("br-lan.10 = %+v", row)
	}
	if row := byName["pppoe-wan"]; !row.WAN || len(row.Networks) != 1 || row.Networks[0] != "wan" || row.Proto != "pppoe" {
		t.Errorf("pppoe-wan = %+v", row)
	}
	if !byName["eth4"].WAN || !byName["eth4.3900"].WAN {
		t.Errorf("WAN ancestry not mapped: eth4=%+v vlan=%+v", byName["eth4"], byName["eth4.3900"])
	}
	if byName["eth4"].RxRate != "13.9 Mbps" || byName["eth2"].TxRate != "14 Mbps" {
		t.Errorf("rates: eth4=%q eth2=%q", byName["eth4"].RxRate, byName["eth2"].TxRate)
	}
}

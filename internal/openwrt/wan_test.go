// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"encoding/binary"
	"testing"
)

func TestDiscoverWANIsPluralNameAgnosticAndExact(t *testing.T) {
	dump := map[string]any{"interface": []any{
		wanStatus("upstream", "eth4.3900", "pppoe-upstream", 20, 4),
		wanStatus("upstream_6", "eth4.3900", "pppoe-upstream", 20, 6),
		wanStatus("backup", "eth3", "eth3", 200, 4),
		map[string]any{
			"interface": "offline", "up": false, "device": "eth2", "l3_device": "eth2",
			"route": []any{map[string]any{"target": "0.0.0.0", "mask": int64(0)}},
		},
	}}

	got := discoverWAN(dump, nil)
	if len(got.Devices) != 2 {
		t.Fatalf("devices = %+v, want two active default-route owners", got.Devices)
	}
	primary, ok := got.Primary()
	if !ok || primary.Device != "pppoe-upstream" || primary.Transport != "eth4.3900" || primary.Uptime != 3600 {
		t.Fatalf("primary = %+v, %v", primary, ok)
	}
	if len(primary.Networks) != 2 || primary.Networks[0] != "upstream" || primary.Networks[1] != "upstream_6" {
		t.Errorf("logical owners = %v", primary.Networks)
	}
	if len(primary.Routes) != 2 || primary.Routes[0].Family != 4 || primary.Routes[1].Family != 6 {
		t.Errorf("family routes = %+v", primary.Routes)
	}
	if got.Devices[1].Device != "eth3" {
		t.Errorf("backup = %+v", got.Devices[1])
	}
}

func TestDiscoverWANSupplementsPolicyAndUnmanagedRoutes(t *testing.T) {
	dump := map[string]any{"interface": []any{
		wanStatus("internet4", "eth0", "eth0", 10, 4),
		wanStatus("internet6", "eth1", "eth1", 10, 6),
		map[string]any{
			"interface": "work-vpn", "up": true, "device": "wg0", "l3_device": "wg0", "uptime": int64(70),
			"route": []any{map[string]any{"target": "0.0.0.0", "mask": int64(0), "table": int64(100)}},
		},
	}}
	kernel := []kernelRoute{
		{Family: 4, Device: "eth0", Table: mainRouteTable, Metric: 10},
		{Family: 6, Device: "eth1", Table: mainRouteTable, Metric: 10},
		{Family: 4, Device: "wg0", Table: 100, Metric: 5},
		{Family: 4, Device: "unmanaged0", Table: 200, Metric: 0},
	}

	got := discoverWAN(dump, kernel)
	if len(got.Devices) != 4 {
		t.Fatalf("devices = %+v, want split-family, policy, and unmanaged WANs", got.Devices)
	}
	byName := map[string]WANDevice{}
	for _, device := range got.Devices {
		byName[device.Device] = device
	}
	if route := byName["wg0"].Routes[0]; route.Main || route.Table != 100 {
		t.Errorf("policy route = %+v", route)
	}
	if networks := byName["wg0"].Networks; len(networks) != 1 || networks[0] != "work-vpn" {
		t.Errorf("policy owner = %v", networks)
	}
	if len(byName["unmanaged0"].Networks) != 0 {
		t.Errorf("unmanaged route invented owners: %+v", byName["unmanaged0"])
	}
}

func TestDiscoverWANWithoutDefaultRouteIsDown(t *testing.T) {
	dump := map[string]any{"interface": []any{map[string]any{
		"interface": "upstream", "up": true, "device": "eth4", "l3_device": "eth4",
		"route": []any{map[string]any{"target": "192.0.2.0", "mask": int64(24)}},
	}}}
	got := discoverWAN(dump, nil)
	if got.Up() || len(got.Devices) != 0 {
		t.Fatalf("state = %+v, want no live WAN", got)
	}
}

func TestPreferredStatusUsesMetricBeforeName(t *testing.T) {
	entries := []any{
		wanStatus("aaa", "eth0", "eth0", 100, 4),
		wanStatus("zzz", "eth1", "eth1", 10, 4),
	}
	got := preferredStatus(entries, 4)
	if got["interface"] != "zzz" {
		t.Fatalf("preferred = %v, want zzz's lower metric", got["interface"])
	}
}

func TestMultipathOutputsSkipsDeadNexthops(t *testing.T) {
	entry := func(flags byte, index uint32) []byte {
		data := make([]byte, 8)
		binary.NativeEndian.PutUint16(data[:2], 8)
		data[2] = flags
		binary.NativeEndian.PutUint32(data[4:8], index)
		return data
	}
	data := append(entry(0, 3), entry(rtnhDead, 4)...)
	data = append(data, entry(rtnhLinkDown, 5)...)
	got := multipathOutputs(data)
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("outputs = %v, want only live ifindex 3", got)
	}
}

func wanStatus(name, transport, l3 string, metric int64, family int) map[string]any {
	target := "0.0.0.0"
	if family == 6 {
		target = "::"
	}
	return map[string]any{
		"interface": name, "up": true, "device": transport, "l3_device": l3,
		"uptime": int64(3600),
		"route":  []any{map[string]any{"target": target, "mask": int64(0), "metric": metric}},
	}
}

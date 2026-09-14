// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkStateNativeFlags(t *testing.T) {
	// The CLI prints booleans, but the native blobmsg decoder returns INT8s.
	// This is the shape from a running netifd, not a JSON-only mock.
	dump := map[string]any{"interface": []any{
		map[string]any{"interface": "lan", "up": int64(1), "pending": int64(0), "autostart": int64(0), "uptime": int64(123),
			"ipv4-address": []any{map[string]any{"address": "192.168.77.1", "mask": int64(24)}}},
	}}
	devices := map[string]any{"br-lan": map[string]any{
		"type": "bridge", "up": int64(1), "carrier": int64(1), "present": int64(1), "mtu": int64(1500),
		"bridge-attributes": map[string]any{"stp": int64(0)}, "statistics": map[string]any{"rx_errors": int64(0)},
	}}
	data, err := json.Marshal(networkState(dump, devices))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	iface := state["interfaces"].([]any)[0].(map[string]any)
	if iface["up"] != true || iface["pending"] != false || iface["autostart"] != false || iface["uptime"] != float64(123) {
		t.Fatalf("bad interface flags: %v", iface)
	}
	device := state["devices"].(map[string]any)["br-lan"].(map[string]any)
	if device["up"] != true || device["carrier"] != true || device["mtu"] != float64(1500) || device["type"] != "bridge" || device["bridge-attributes"].(map[string]any)["stp"] != false || device["statistics"].(map[string]any)["rx_errors"] != float64(0) {
		t.Fatalf("bad device facts: %v", device)
	}
}

func TestUnmanagedPortUsesKernelFacts(t *testing.T) {
	root := t.TempDir()
	port := filepath.Join(root, "test-port")
	if err := os.MkdirAll(filepath.Join(port, "statistics"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, value := range map[string]string{"flags": "0x1002", "carrier": "0", "address": "02:11:22:33:44:55", "mtu": "1500", "operstate": "down", "speed": "-1", "statistics/rx_errors": "3", "statistics/tx_errors": "0"} {
		if err := os.WriteFile(filepath.Join(port, file), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	devices := map[string]any{}
	kernelNetworkDevices(root, devices)
	d := devices["test-port"].(map[string]any)
	if d["up"] != false || d["carrier"] != false || d["present"] != true || d["macaddr"] != "02:11:22:33:44:55" || d["mtu"] != uint64(1500) {
		t.Fatalf("bad unmanaged device: %v", d)
	}
	if _, ok := d["speed"]; ok {
		t.Fatal("invented a speed from an unavailable reading")
	}
	if d["statistics"].(map[string]any)["rx_errors"] != uint64(3) {
		t.Fatal("lost actual error counter")
	}
	d["up"] = true
	kernelNetworkDevices(root, devices)
	if d["up"] != true {
		t.Fatal("kernel fallback overwrote netifd's own state")
	}
}

func TestKernelLinkAddressesAreNotAllMACs(t *testing.T) {
	for _, tc := range []struct{ name, kind, address, reported, want string }{
		{"ip6tnl0", "769", "00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00", "", ""},
		{"configured-ip6-tunnel", "769", "20:01:0d:b8:00:00:00:00:00:00:00:00:00:00:00:01", "", ""},
		{"sit0", "776", "0.0.0.0", "", ""},
		{"ppp0", "512", "", "", ""},
		{"empty-ethernet", "1", "00:00:00:00:00:00", "", ""},
		{"br-lan", "1", "02:11:22:33:44:55", "", "02:11:22:33:44:55"},
		{"gretap0", "1", "02:11:22:33:44:66", "", "02:11:22:33:44:66"},
		{"netifd-wins", "1", "02:11:22:33:44:55", "02:11:22:33:44:77", "02:11:22:33:44:77"},
		{"netifd-non-mac", "769", "::", "00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, tc.name)
			if err := os.Mkdir(base, 0700); err != nil {
				t.Fatal(err)
			}
			for file, value := range map[string]string{"type": tc.kind, "address": tc.address} {
				if err := os.WriteFile(filepath.Join(base, file), []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			devices := map[string]any{tc.name: map[string]any{"macaddr": tc.reported}}
			kernelNetworkDevices(root, devices)
			device := devices[tc.name].(map[string]any)
			got, _ := device["macaddr"].(string)
			if got != tc.want {
				t.Fatalf("MAC = %q, want %q", got, tc.want)
			}
			if tc.want == "" {
				if _, exists := device["macaddr"]; exists {
					t.Fatal("non-MAC address must be absent")
				}
			}
		})
	}
}

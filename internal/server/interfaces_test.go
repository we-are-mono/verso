// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "testing"

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

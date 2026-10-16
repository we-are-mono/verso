// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestNetIfacesCarryTheirAddresses: each logical interface says the addresses
// it holds now, both families, without their prefix length, so the shell can
// tell which interface a browser reaches the router through.
func TestNetIfacesCarryTheirAddresses(t *testing.T) {
	var dump map[string]any
	if err := json.Unmarshal([]byte(`{"interface": [
		{"interface": "lan", "up": true, "l3_device": "br-lan", "ipv4-address": [{"address": "192.168.77.1", "mask": 24}], "ipv6-address": []},
		{"interface": "wan6", "up": true, "l3_device": "wan0", "ipv4-address": [], "ipv6-address": [{"address": "fd42:7ea:1::10", "mask": 64}]},
		{"interface": "guest", "up": false, "device": "br-guest"}
	]}`), &dump); err != nil {
		t.Fatal(err)
	}
	got := parseNetIfaces(dump)
	want := []NetIface{
		{Name: "lan", Device: "br-lan", Up: true, Addresses: []string{"192.168.77.1"}},
		{Name: "wan6", Device: "wan0", Up: true, Addresses: []string{"fd42:7ea:1::10"}},
		{Name: "guest", Device: "br-guest"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNetIfaces = %+v, want %+v", got, want)
	}
}

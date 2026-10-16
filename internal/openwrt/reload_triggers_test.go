// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestReloadTriggersFromOpenWrt25: procd's own table says which init scripts
// a change to each config reloads. Read from a lab router's `service list
// {"verbose": true}`: dhcp reloads dnsmasq, nlbwmon and odhcpd; wireless
// reloads network; a script that is not an init script (ucitrack's ubus
// call) reloads no service; a service's script is named by its own path,
// not by the service that registered it.
func TestReloadTriggersFromOpenWrt25(t *testing.T) {
	raw, err := os.ReadFile("testdata/service-list-25.12.json")
	if err != nil {
		t.Fatal(err)
	}
	var table map[string]any
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	got := reloadTriggers(table)
	for config, want := range map[string][]string{
		"dhcp":     {"dnsmasq", "nlbwmon", "odhcpd"},
		"firewall": {"firewall", "packet_steering"},
		"network":  {"network", "odhcpd", "packet_steering"},
		"wireless": {"network"},
		"dropbear": {"dropbear"},
		"adblock":  {"adblock"},
	} {
		if !reflect.DeepEqual(got[config], want) {
			t.Errorf("%s reloads %v, want %v", config, got[config], want)
		}
	}
	for _, service := range got["firewall"] {
		if service == "ubus" || service == "ucitrack" {
			t.Errorf("firewall lists %q, which no init script is", service)
		}
	}
}

// TestReloadTriggersOfOtherShapes: a trigger whose condition names its
// package inside an "or" counts for each package; one whose shape is unknown
// (no condition, a condition on something else, an action that is no init
// script) reloads nothing.
func TestReloadTriggersOfOtherShapes(t *testing.T) {
	var table map[string]any
	if err := json.Unmarshal([]byte(`{
		"multi": {"triggers": [["config.change", ["if", ["or", ["eq", "package", "a"], ["eq", "package", "b"]], ["run_script", "/etc/init.d/multi", "reload"]], 1000]]},
		"bare": {"triggers": [["config.change", ["run_script", "/etc/init.d/bare", "reload"]]]},
		"iface": {"triggers": [["interface.*", ["if", ["eq", "interface", "lan"], ["run_script", "/etc/init.d/iface", "reload"]], 1000]]},
		"odd": {"triggers": [["config.change", "not a list"], 42, ["config.change", ["if", ["eq", "package", "c"], ["run_script", "/usr/bin/thing"]]]]},
		"none": {"instances": {}}
	}`), &table); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"a": {"multi"}, "b": {"multi"}}
	if got := reloadTriggers(table); !reflect.DeepEqual(got, want) {
		t.Errorf("reloadTriggers = %v, want %v", got, want)
	}
}

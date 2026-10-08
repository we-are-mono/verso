// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"slices"
	"testing"
)

// stockFirewall is OpenWrt's shipped firewall, its Allow-Ping and DHCP rules
// among the rest: nothing in it reaches the shell from the internet.
func stockFirewall() map[string]any {
	return map[string]any{
		"defaults": map[string]any{".type": "defaults", "input": "REJECT"},
		"lan":      map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}, "input": "ACCEPT"},
		"wanz":     map[string]any{".type": "zone", "name": "wan", "network": []any{"wan", "wan6"}, "input": "REJECT", "masq": "1"},
		"ping":     map[string]any{".type": "rule", "name": "Allow-Ping", "src": "wan", "proto": "icmp", "icmp_type": "echo-request", "target": "ACCEPT"},
		"dhcp":     map[string]any{".type": "rule", "name": "Allow-DHCP-Renew", "src": "wan", "proto": "udp", "dest_port": "68", "target": "ACCEPT"},
		"fwd":      map[string]any{".type": "rule", "name": "Allow-IPSec-ESP", "src": "wan", "dest": "lan", "proto": "esp", "target": "ACCEPT"},
	}
}

func with(fw map[string]any, name string, section map[string]any) map[string]any {
	fw[name] = section
	return fw
}

func TestTheStockFirewallKeepsTheShellOffTheInternet(t *testing.T) {
	if got := exposure(stockFirewall(), []string{"wan", "wan6"}, []string{"443", "80"}); len(got) != 0 {
		t.Errorf("stock firewall reported %v", got)
	}
}

func TestAWANZoneThatAcceptsInputExposesTheShell(t *testing.T) {
	fw := stockFirewall()
	fw["wanz"].(map[string]any)["input"] = "ACCEPT"
	got := exposure(fw, nil, []string{"443"})
	if len(got) != 1 || got[0].kind != exposedZone || got[0].name != "wan" {
		t.Errorf("got %v, want the wan zone", got)
	}
	// A zone with no input of its own takes the defaults'.
	fw = stockFirewall()
	delete(fw["wanz"].(map[string]any), "input")
	fw["defaults"].(map[string]any)["input"] = "ACCEPT"
	if got := exposure(fw, nil, []string{"443"}); len(got) != 1 {
		t.Errorf("got %v, want the defaults' accept to count", got)
	}
}

func TestARuleAcceptingTheShellsPortFromAWANZoneExposesIt(t *testing.T) {
	cases := []struct {
		rule map[string]any
		want bool
	}{
		{map[string]any{"src": "wan", "dest_port": "443", "target": "ACCEPT"}, true},
		{map[string]any{"src": "wan", "dest_port": "400-500", "proto": "tcp", "target": "ACCEPT"}, true},
		{map[string]any{"src": "wan", "dest_port": "8000-9000", "proto": "tcp", "target": "ACCEPT"}, false},
		{map[string]any{"src": "*", "dest_port": "80 443", "target": "ACCEPT"}, true},
		{map[string]any{"src": "wan", "target": "ACCEPT"}, true}, // every port
		{map[string]any{"src": "wan", "dest_port": "443", "proto": "udp", "target": "ACCEPT"}, false},
		{map[string]any{"src": "wan", "dest_port": "443"}, false}, // a rule's own default is DROP
		{map[string]any{"src": "wan", "dest_port": "443", "target": "ACCEPT", "enabled": "0"}, false},
		{map[string]any{"src": "wan", "dest": "lan", "dest_port": "443", "target": "ACCEPT"}, false}, // forwarded, not to the router
		{map[string]any{"src": "lan", "dest_port": "443", "target": "ACCEPT"}, false},
		{map[string]any{"src": "wan", "dest_port": "22", "target": "ACCEPT"}, false},
	}
	for _, c := range cases {
		rule := map[string]any{".type": "rule", "name": "Admin"}
		for k, v := range c.rule {
			rule[k] = v
		}
		got := exposure(with(stockFirewall(), "admin", rule), nil, []string{"443", "80"})
		if (len(got) == 1 && got[0].kind == exposedRule && got[0].name == "Admin") != c.want {
			t.Errorf("rule %v: got %v, want exposed=%v", c.rule, got, c.want)
		}
	}
}

func TestAPortForwardToTheRouterItselfExposesTheShell(t *testing.T) {
	cases := []struct {
		redirect map[string]any
		want     bool
	}{
		{map[string]any{"src": "wan", "src_dport": "8443"}, true}, // no dest_ip: the router
		{map[string]any{"src": "wan", "src_dport": "9443", "dest_port": "8443"}, true},
		{map[string]any{"src": "wan", "src_dport": "8443", "dest_ip": "192.168.1.20"}, false},
		{map[string]any{"src": "wan", "src_dport": "8443", "enabled": "0"}, false},
		{map[string]any{"src": "wan", "src_dport": "8443", "target": "SNAT"}, false},
	}
	for _, c := range cases {
		redirect := map[string]any{".type": "redirect"}
		for k, v := range c.redirect {
			redirect[k] = v
		}
		got := exposure(with(stockFirewall(), "fwd8443", redirect), nil, []string{"8443"})
		if (len(got) == 1 && got[0].kind == exposedForward && got[0].name == "fwd8443") != c.want {
			t.Errorf("redirect %v: got %v, want exposed=%v", c.redirect, got, c.want)
		}
	}
}

func TestAZoneHoldingAWANNetworkIsAWANZoneWhateverItsName(t *testing.T) {
	fw := with(stockFirewall(), "up", map[string]any{".type": "zone", "name": "uplink", "network": "pppoe lte", "input": "ACCEPT"})
	if got := exposure(fw, nil, []string{"443"}); len(got) != 0 {
		t.Errorf("a zone holding no WAN network was called a WAN zone: %v", got)
	}
	got := exposure(fw, []string{"lte"}, []string{"443"})
	names := []string{}
	for _, e := range got {
		names = append(names, e.name)
	}
	if !slices.Contains(names, "uplink") {
		t.Errorf("got %v, want the zone holding the WAN network", got)
	}
}

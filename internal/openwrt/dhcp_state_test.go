// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"errors"
	"testing"
)

func dhcpFixture() (map[string]any, map[string]any, map[string]any, map[string]string) {
	config := map[string]any{"lan": map[string]any{".type": "dhcp", "interface": "lan"}}
	instance := map[string]any{"running": int64(1), "pid": int64(123), "command": []any{"/usr/sbin/dnsmasq", "-C", "/var/etc/dnsmasq.conf.lan", "-k"}}
	services := map[string]any{"dnsmasq": map[string]any{"instances": map[string]any{"main": instance}}}
	dump := map[string]any{"interface": []any{map[string]any{"interface": "lan", "up": int64(1), "l3_device": "br-lan", "ipv4-address": []any{map[string]any{"address": "192.168.1.1", "mask": int64(24)}}}}}
	files := map[string]string{"/var/etc/dnsmasq.conf.lan": "dhcp-leasefile=/tmp/dhcp.leases\ndhcp-range=set:lan,192.168.1.100,192.168.1.249,255.255.255.0,12h\n", "/tmp/dhcp.leases": "2000 aa:bb:cc:dd:ee:01 192.168.1.101 laptop *\n900 aa:bb:cc:dd:ee:02 192.168.1.102 expired *\n0 aa:bb:cc:dd:ee:03 192.168.1.50 reserved *\n2000 aa:bb:cc:dd:ee:04 192.168.2.100 other *\n2000 aa:bb:cc:dd:ee:01 192.168.1.101 duplicate *\n"}
	return config, services, dump, files
}
func TestDHCPObservedServiceStates(t *testing.T) {
	for _, tc := range []struct {
		name, state, reason string
		change              func(map[string]any, map[string]any, map[string]any, map[string]string)
	}{
		{"running", "running", "", func(_, _, _ map[string]any, _ map[string]string) {}},
		{"stopped", "stopped", "", func(_, s, _ map[string]any, _ map[string]string) {
			s["dnsmasq"].(map[string]any)["instances"].(map[string]any)["main"].(map[string]any)["running"] = int64(0)
		}},
		{"failed", "stopped", "failed", func(_, s, _ map[string]any, _ map[string]string) {
			v := s["dnsmasq"].(map[string]any)["instances"].(map[string]any)["main"].(map[string]any)
			v["running"] = false
			v["exit_code"] = int64(1)
		}},
		{"down interface", "warning", "interface-down", func(_, _, d map[string]any, _ map[string]string) {
			d["interface"].([]any)[0].(map[string]any)["up"] = false
		}},
		{"different active subnet", "warning", "not-serving", func(_, _, d map[string]any, _ map[string]string) {
			d["interface"].([]any)[0].(map[string]any)["ipv4-address"].([]any)[0].(map[string]any)["address"] = "192.168.2.1"
		}},
		{"range suppressed", "warning", "not-serving", func(_, _, _ map[string]any, f map[string]string) {
			f["/var/etc/dnsmasq.conf.lan"] = "no-dhcp-interface=br-lan\n"
		}},
		{"excluded interface", "warning", "not-serving", func(_, _, _ map[string]any, f map[string]string) {
			f["/var/etc/dnsmasq.conf.lan"] += "no-dhcp-interface=br-*\n"
		}},
		{"unreadable config", "warning", "unknown", func(_, _, _ map[string]any, f map[string]string) { delete(f, "/var/etc/dnsmasq.conf.lan") }},
		{"unreadable leases", "warning", "leases-unavailable", func(_, _, _ map[string]any, f map[string]string) { delete(f, "/tmp/dhcp.leases") }},
		{"disabled", "disabled", "", func(c, _, _ map[string]any, f map[string]string) {
			c["lan"].(map[string]any)["ignore"] = "1"
			f["/var/etc/dnsmasq.conf.lan"] = ""
		}},
		{"full pool", "warning", "pool-full", func(_, _, _ map[string]any, f map[string]string) {
			f["/var/etc/dnsmasq.conf.lan"] = "dhcp-range=set:lan,192.168.1.101,192.168.1.101,255.255.255.0,12h\n"
		}},
		{"reservations only", "running", "", func(_, _, _ map[string]any, f map[string]string) {
			f["/var/etc/dnsmasq.conf.lan"] = "dhcp-range=tag:trusted,set:lan,192.168.1.100,static,255.255.255.0,12h\n"
		}},
		{"wrong instance cannot borrow health", "stopped", "", func(c, s, _ map[string]any, _ map[string]string) {
			c["lan"].(map[string]any)["instance"] = "other"
			s["dnsmasq"].(map[string]any)["instances"].(map[string]any)["other"] = map[string]any{"running": false}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, s, d, f := dhcpFixture()
			tc.change(c, s, d, f)
			states := dhcpNetworkStates(c, s, d, func(path string) ([]byte, error) {
				if v, ok := f[path]; ok {
					return []byte(v), nil
				}
				return nil, errors.New("unreadable")
			}, 1000)
			if state := states["lan"]; state.State != tc.state || state.Reason != tc.reason {
				t.Fatalf("got %+v, want %s/%s", state, tc.state, tc.reason)
			}
		})
	}
}
func TestDHCPLeasesUseSubnetExpiryAndUniqueAddresses(t *testing.T) {
	c, s, d, f := dhcpFixture()
	states := dhcpNetworkStates(c, s, d, func(path string) ([]byte, error) { return []byte(f[path]), nil }, 1000)
	state := states["lan"]
	if state.Leases == nil || *state.Leases != 2 || state.Pool != "192.168.1.100–192.168.1.249" || state.LeaseTime != "12h" {
		t.Fatalf("wrong live facts: %+v", state)
	}
}

func TestDHCPIncludesCanSuppressAGeneratedRange(t *testing.T) {
	c, s, d, f := dhcpFixture()
	f["/var/etc/dnsmasq.conf.lan"] += "conf-file=/etc/dnsmasq.conf\n"
	f["/etc/dnsmasq.conf"] = "no-dhcp-interface=br-lan\n"
	reader := func(path string) ([]byte, error) {
		body, ok := f[path]
		if !ok {
			return nil, errors.New("missing file")
		}
		return []byte(body), nil
	}
	state := dhcpNetworkStates(c, s, d, reader, 1000)["lan"]
	if state.State != "warning" || state.Reason != "not-serving" {
		t.Fatalf("ignored custom exclusion: %+v", state)
	}
	delete(f, "/etc/dnsmasq.conf")
	state = dhcpNetworkStates(c, s, d, reader, 1000)["lan"]
	if state.State != "warning" || state.Reason != "unknown" {
		t.Fatalf("unreadable include reported healthy: %+v", state)
	}
}

func dhcpNetworkStates(config, services, dump map[string]any, read func(string) ([]byte, error), now int64) map[string]dhcpNetworkState {
	return dhcpStates(config, services, dump, read, listDHCPFiles, now)
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net"
	"strings"
)

// Zones: which firewall zone a device lives behind. The firewall config
// names zones over networks; the network config gives each interface its
// subnet — joining the two (interfaceNets, networkZones) maps any address to
// its zone. That is the truth the roster's zone chip states: the wall this
// device sits behind.

// zonesByDevice maps kernel interface names to their firewall zone through the
// UCI interface's configured device. It covers link-local and delegated IPv6
// neighbours that cannot be classified by a static address subnet.
func zonesByDevice(netCfg, fwCfg map[string]any) map[string]string {
	zoneOf := networkZones(fwCfg)
	out := map[string]string{}
	for _, value := range netCfg {
		section, ok := value.(map[string]any)
		if !ok || section[".type"] != "interface" {
			continue
		}
		name, _ := section[".name"].(string)
		device, _ := section["device"].(string)
		if device != "" && zoneOf[name] != "" {
			out[device] = zoneOf[name]
		}
	}
	return out
}

// staticSubnets are the IPv4 subnets an interface section's static addresses
// stand on, in the order written. netifd takes ipaddr as one address or a list,
// each with its prefix ("10.0.0.1/24") or under the section's dotted netmask;
// a prefix wins over the netmask, and an address with neither has no subnet.
func staticSubnets(section map[string]any) []*net.IPNet {
	netmask, _ := section["netmask"].(string)
	var out []*net.IPNet
	for _, addr := range uciList(section["ipaddr"]) {
		if n := subnetOf(addr, netmask); n != nil {
			out = append(out, n)
		}
	}
	return out
}

// subnetOf turns one uci static IPv4 address, prefixed or under a dotted
// netmask, into its subnet.
func subnetOf(ipaddr, netmask string) *net.IPNet {
	if strings.Contains(ipaddr, "/") {
		ip, n, err := net.ParseCIDR(ipaddr)
		if err != nil || ip.To4() == nil {
			return nil
		}
		return n
	}
	ip := net.ParseIP(ipaddr).To4()
	maskIP := net.ParseIP(netmask)
	if ip == nil || maskIP == nil {
		return nil
	}
	mask := net.IPMask(maskIP.To4())
	if mask == nil {
		return nil
	}
	return &net.IPNet{IP: ip.Mask(mask), Mask: mask}
}

// uciList reads a uci option that may be a list or a single string.
func uciList(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

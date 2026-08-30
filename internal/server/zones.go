// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net"
)

// Zones: which firewall zone a device lives behind. The firewall config
// names zones over networks; the network config gives each interface its
// subnet — joining the two maps any address to its zone. That is the truth
// the roster's zone chip states: the wall this device sits behind.

// zoneNet is one interface subnet and the zone that covers it.
type zoneNet struct {
	cidr *net.IPNet
	zone string
}

func zoneNets(netCfg, fwCfg map[string]any) []zoneNet {
	zoneOf := networkZones(fwCfg)
	var zones []zoneNet
	for _, v := range netCfg {
		section, ok := v.(map[string]any)
		if !ok || section[".type"] != "interface" {
			continue
		}
		name, _ := section[".name"].(string)
		zone := zoneOf[name]
		if zone == "" {
			continue
		}
		ipaddr, _ := section["ipaddr"].(string)
		netmask, _ := section["netmask"].(string)
		if cidr := subnetOf(ipaddr, netmask); cidr != nil {
			zones = append(zones, zoneNet{cidr: cidr, zone: zone})
		}
	}
	return zones
}

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

// zoneForAddr names the zone whose subnet holds addr, or "".
func zoneForAddr(zones []zoneNet, addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return ""
	}
	for _, z := range zones {
		if z.cidr.Contains(ip) {
			return z.zone
		}
	}
	return ""
}

// subnetOf turns a uci static address + dotted netmask into its subnet.
func subnetOf(ipaddr, netmask string) *net.IPNet {
	ip := net.ParseIP(ipaddr)
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

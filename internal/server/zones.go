// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
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

// zoneMap builds the address→zone lookup from live config. Empty on any
// trouble — a chip is decoration, never worth an error.
func (s *Server) zoneMap(ctx context.Context, sid string) []zoneNet {
	netCfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: zones: network config unavailable: %v", err)
		return nil
	}
	fwCfg, err := s.backend.UCIConfig(ctx, sid, "firewall")
	if err != nil {
		log.Printf("verso: zones: firewall config unavailable: %v", err)
		return nil
	}

	// interface name → zone name, from the firewall's zone sections.
	zoneOf := make(map[string]string)
	for _, v := range fwCfg {
		section, ok := v.(map[string]any)
		if !ok || section[".type"] != "zone" {
			continue
		}
		zone, _ := section["name"].(string)
		if zone == "" {
			continue
		}
		for _, network := range uciList(section["network"]) {
			zoneOf[network] = zone
		}
	}

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

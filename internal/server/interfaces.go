// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
)

// Interfaces: the logical-network map between the physical ports and the clients.
// Each `config interface` with an address is an interface; a bridge-VLAN gives it
// its 802.1Q id and the tagged/untagged ports it rides. Joining that with the
// firewall zones and the device roster answers the question LuCI leaves scattered
// across pages: what networks exist, on which ports, behind which wall, with how
// many devices.

// ifaceNet is one interface's subnet and the interface name that owns it — the
// finer-grained twin of zoneNet, used to name the network a device sits on.
type ifaceNet struct {
	cidr   *net.IPNet
	name   string
	device string
}

// interfaceNets builds the address→interface lookup from the network config: each
// addressed `config interface` and its subnet.
func interfaceNets(cfg map[string]any) []ifaceNet {
	var out []ifaceNet
	for _, v := range cfg {
		s := asSection(v)
		if s == nil || s[".type"] != "interface" {
			continue
		}
		name, _ := s[".name"].(string)
		device, _ := s["device"].(string)
		ipaddr, _ := s["ipaddr"].(string)
		netmask, _ := s["netmask"].(string)
		if c := subnetOf(ipaddr, netmask); c != nil {
			out = append(out, ifaceNet{cidr: c, name: name, device: device})
		}
	}
	return out
}

// ifaceForAddr names the interface whose subnet holds addr, or "".
func ifaceForAddr(nets []ifaceNet, addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return ""
	}
	for _, n := range nets {
		if n.cidr.Contains(ip) {
			if n.device != "" {
				return n.device
			}
			return n.name
		}
	}
	return ""
}

// deviceVLAN splits an interface's device into its bridge and VLAN id: "br-lan.20"
// → ("br-lan", "20"); a plain device → (device, "").
func deviceVLAN(device string) (bridge, id string) {
	if i := strings.LastIndexByte(device, '.'); i >= 0 {
		if _, err := strconv.Atoi(device[i+1:]); err == nil {
			return device[:i], device[i+1:]
		}
	}
	return device, ""
}

// networkZones maps each network name to its firewall zone.
func networkZones(fwCfg map[string]any) map[string]string {
	out := map[string]string{}
	for _, v := range fwCfg {
		s, ok := v.(map[string]any)
		if !ok || s[".type"] != "zone" {
			continue
		}
		zone, _ := s["name"].(string)
		if zone == "" {
			continue
		}
		for _, network := range uciList(s["network"]) {
			out[network] = zone
		}
	}
	return out
}

type interfaceMeaning struct {
	name, proto, subnet, zone, vlan string
}

// interfaceMeanings indexes UCI's logical networks by their configured kernel
// device. Telemetry remains responsible for which interfaces exist.
func interfaceMeanings(cfg, fwCfg map[string]any) map[string][]interfaceMeaning {
	zoneOf := networkZones(fwCfg)
	out := map[string][]interfaceMeaning{}
	for _, value := range cfg {
		section := asSection(value)
		if section == nil || section[".type"] != "interface" {
			continue
		}
		name, _ := section[".name"].(string)
		device, _ := section["device"].(string)
		proto, _ := section["proto"].(string)
		if device == "" {
			continue
		}
		ipaddr, _ := section["ipaddr"].(string)
		netmask, _ := section["netmask"].(string)
		subnet := ""
		if cidr := subnetOf(ipaddr, netmask); cidr != nil {
			subnet = cidr.String()
		}
		_, vlan := deviceVLAN(device)
		out[device] = append(out[device], interfaceMeaning{
			name: name, proto: proto, subnet: subnet,
			zone: zoneOf[name], vlan: vlan,
		})
	}
	return out
}

// interfaceList builds one row per kernel netdev from the process-wide
// telemetry snapshot, then enriches those rows with UCI and WAN topology.
func (s *Server) interfaceList(ctx context.Context, sid string, snapshot telemetry.Snapshot) []widget.OverviewInterface {
	cfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: interfaces: network config unavailable: %v", err)
		cfg = map[string]any{}
	}
	fwCfg, err := s.backend.UCIConfig(ctx, sid, "firewall")
	if err != nil {
		log.Printf("verso: interfaces: firewall config unavailable: %v", err)
		fwCfg = map[string]any{}
	}
	meanings := interfaceMeanings(cfg, fwCfg)
	physical := make(map[string]bool, len(snapshot.Interfaces))
	parents := make(map[string]string, len(snapshot.Interfaces))
	for _, iface := range snapshot.Interfaces {
		physical[iface.Name], parents[iface.Name] = iface.Physical, iface.Parent
	}

	// A protocol-created L3 device (notably pppoe-wan) inherits the configured
	// WAN device's UCI meaning and points back to that underlay.
	wanDevice := ""
	if wan, wanErr := s.backend.WANStatus(ctx, sid); wanErr == nil {
		wanDevice = wan.Device
	}
	configuredWAN := ""
	for device, list := range meanings {
		for _, meaning := range list {
			if meaning.name == "wan" {
				configuredWAN = device
				break
			}
		}
		if configuredWAN != "" {
			break
		}
	}
	if wanDevice != "" && configuredWAN != "" && wanDevice != configuredWAN {
		for _, meaning := range meanings[configuredWAN] {
			if meaning.name == "wan" {
				meanings[wanDevice] = append(meanings[wanDevice], meaning)
				break
			}
		}
		if parents[wanDevice] == "" {
			parents[wanDevice] = configuredWAN
		}
	}
	if wanDevice == "" {
		wanDevice = configuredWAN
	}

	// Relationships are bidirectional in the presentation: a VLAN names its
	// parent, and the physical parent names that attached VLAN; bridge members
	// likewise point back to their bridge.
	relations := make(map[string]map[string]bool, len(snapshot.Interfaces))
	addRelation := func(from, to string) {
		if from == "" || to == "" || from == to {
			return
		}
		if relations[from] == nil {
			relations[from] = map[string]bool{}
		}
		relations[from][to] = true
	}
	for _, iface := range snapshot.Interfaces {
		if parent := parents[iface.Name]; parent != "" {
			addRelation(iface.Name, parent)
			addRelation(parent, iface.Name)
		}
		for _, member := range iface.Members {
			addRelation(iface.Name, member)
			addRelation(member, iface.Name)
		}
	}

	wanPath := map[string]bool{}
	for current, seen := wanDevice, map[string]bool{}; current != "" && !seen[current]; current = parents[current] {
		seen[current], wanPath[current] = true, true
	}

	out := make([]widget.OverviewInterface, 0, len(snapshot.Interfaces))
	for _, iface := range snapshot.Interfaces {
		row := widget.OverviewInterface{
			Name: iface.Name, Kind: iface.Kind, State: normalOperstate(iface.Operstate),
			Physical: iface.Physical, WAN: wanPath[iface.Name],
		}
		if len(iface.History) != 0 {
			point := iface.History[len(iface.History)-1]
			row.RxRate, row.TxRate = formatBitRate(point.RxBPS), formatBitRate(point.TxBPS)
			row.RxPackets, row.TxPackets = formatPacketRate(point.RxPPS), formatPacketRate(point.TxPPS)
			row.RxTotal, row.TxTotal = formatCounterBytes(point.RxBytes), formatCounterBytes(point.TxBytes)
		}
		seenNetworks := map[string]bool{}
		for _, meaning := range meanings[iface.Name] {
			if meaning.name != "" && !seenNetworks[meaning.name] {
				row.Networks = append(row.Networks, meaning.name)
				seenNetworks[meaning.name] = true
			}
			if row.Proto == "" {
				row.Proto = meaning.proto
			}
			if row.Subnet == "" {
				row.Subnet = meaning.subnet
			}
			if row.Zone == "" {
				row.Zone = meaning.zone
			}
			if row.VLAN == "" {
				row.VLAN = meaning.vlan
			}
		}
		sort.Strings(row.Networks)
		for name := range relations[iface.Name] {
			row.Relations = append(row.Relations, widget.OverviewInterfaceRelation{Name: name, Physical: physical[name]})
		}
		sort.Slice(row.Relations, func(i, j int) bool {
			if row.Relations[i].Physical != row.Relations[j].Physical {
				return row.Relations[i].Physical
			}
			return row.Relations[i].Name < row.Relations[j].Name
		})
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := interfaceKindRank(out[i].Kind), interfaceKindRank(out[j].Kind)
		if left != right {
			return left < right
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func normalOperstate(state string) string {
	switch state {
	case "up", "down":
		return state
	default:
		return "unknown"
	}
}

func interfaceKindRank(kind string) int {
	switch kind {
	case "port":
		return 0
	case "bridge":
		return 1
	case "vlan":
		return 2
	case "pppoe":
		return 3
	case "wifi":
		return 4
	case "tunnel":
		return 5
	case "virtual":
		return 6
	default:
		return 7
	}
}

func formatBitRate(bits uint64) string {
	switch {
	case bits >= 1_000_000_000:
		return trimOneDecimal(float64(bits)/1_000_000_000) + " Gbps"
	case bits >= 1_000_000:
		return trimOneDecimal(float64(bits)/1_000_000) + " Mbps"
	case bits >= 1_000:
		return trimOneDecimal(float64(bits)/1_000) + " Kbps"
	default:
		return strconv.FormatUint(bits, 10) + " bps"
	}
}

func formatPacketRate(packets uint64) string {
	return strconv.FormatUint(packets, 10) + " pkt/s"
}

func formatCounterBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	index := 0
	for value >= unit && index < len("KMGTPE") {
		value /= unit
		index++
	}
	return fmt.Sprintf("%.1f %ciB", value, "KMGTPE"[index-1])
}

func trimOneDecimal(value float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0")
}

// asSection narrows a uci config value to a section map.
func asSection(v any) map[string]any {
	s, _ := v.(map[string]any)
	return s
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

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
	cidr *net.IPNet
	name string
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
		ipaddr, _ := s["ipaddr"].(string)
		netmask, _ := s["netmask"].(string)
		if c := subnetOf(ipaddr, netmask); c != nil {
			out = append(out, ifaceNet{cidr: c, name: name})
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
			return n.name
		}
	}
	return ""
}

// vlanMember is one port's membership in a bridge VLAN — tagged (a trunk) or
// untagged (an access port).
type vlanMember struct {
	port   string
	tagged bool
}

// bridgeVLAN is one `config bridge-vlan`: the bridge it sits on, its 802.1Q id,
// and its member ports.
type bridgeVLAN struct {
	bridge  string
	id      string
	members []vlanMember
}

// parseBridgeVLANs reads every `config bridge-vlan` (the DSA VLAN model).
func parseBridgeVLANs(cfg map[string]any) []bridgeVLAN {
	var out []bridgeVLAN
	for _, v := range cfg {
		s, ok := v.(map[string]any)
		if !ok || s[".type"] != "bridge-vlan" {
			continue
		}
		bridge, _ := s["device"].(string)
		bv := bridgeVLAN{bridge: bridge, id: uciScalar(s["vlan"])}
		for _, p := range uciList(s["ports"]) {
			name, tagged := parseVLANPort(p)
			bv.members = append(bv.members, vlanMember{port: name, tagged: tagged})
		}
		out = append(out, bv)
	}
	return out
}

// parseVLANPort splits a DSA port spec ("lan1", "lan2:t", "lan1:u*") into the
// port name and whether the VLAN is tagged on it ("t").
func parseVLANPort(spec string) (name string, tagged bool) {
	name = spec
	if i := strings.IndexByte(spec, ':'); i >= 0 {
		name, tagged = spec[:i], strings.Contains(spec[i+1:], "t")
	}
	return name, tagged
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

// portVLANs maps each port to the VLAN ids it carries (for the Interfaces
// column): "20, 30" or "" when the port carries no tagged VLAN.
func portVLANs(vlans []bridgeVLAN) map[string]string {
	byPort := map[string][]string{}
	for _, bv := range vlans {
		if bv.id == "" {
			continue
		}
		for _, m := range bv.members {
			byPort[m.port] = append(byPort[m.port], bv.id)
		}
	}
	out := make(map[string]string, len(byPort))
	for port, ids := range byPort {
		sort.Slice(ids, func(i, j int) bool { return numLess(ids[i], ids[j]) })
		out[port] = strings.Join(ids, ", ")
	}
	return out
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

// bridgePortsOf returns the member ports declared on a `config device` bridge.
func bridgePortsOf(cfg map[string]any, bridge string) []string {
	for _, v := range cfg {
		s, ok := v.(map[string]any)
		if !ok || s[".type"] != "device" {
			continue
		}
		if name, _ := s["name"].(string); name == bridge {
			return uciList(s["ports"])
		}
	}
	return nil
}

// interfaceList builds the Networks roster: one segment per addressed interface,
// with its VLAN, subnet, zone, ports, and device count. devices is the roster
// already read for the Connected-devices table, so the count needs no re-read.
func (s *Server) interfaceList(ctx context.Context, sid string, devices []widget.OverviewDevice) []widget.OverviewInterface {
	cfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: networks: network config unavailable: %v", err)
		return nil
	}
	fwCfg, err := s.backend.UCIConfig(ctx, sid, "firewall")
	if err != nil {
		log.Printf("verso: networks: firewall config unavailable: %v", err)
	}
	vlans := parseBridgeVLANs(cfg)
	zoneOf := networkZones(fwCfg)

	seen := map[string]bool{}
	var out []widget.OverviewInterface
	for _, v := range cfg {
		s := asSection(v)
		if s == nil || s[".type"] != "interface" {
			continue
		}
		name, _ := s[".name"].(string)
		device, _ := s["device"].(string)
		proto, _ := s["proto"].(string)
		ipaddr, _ := s["ipaddr"].(string)
		netmask, _ := s["netmask"].(string)

		// A network worth a row is an addressed segment — a static subnet or a
		// dynamic uplink — never loopback, and only once per underlying device
		// (so wan and its wan6 twin fold into one row).
		if name == "loopback" || device == "" {
			continue
		}
		if ipaddr == "" && !isUplinkProto(proto) {
			continue
		}
		if seen[device] {
			continue
		}
		seen[device] = true

		bridge, id := deviceVLAN(device)
		subnet := ""
		if c := subnetOf(ipaddr, netmask); c != nil {
			subnet = c.String()
		}
		portDetail := interfacePorts(cfg, vlans, bridge, id, device)

		out = append(out, widget.OverviewInterface{
			Name:       name,
			VLAN:       id,
			Subnet:     subnet,
			Zone:       zoneOf[name],
			Ports:      portSummary(portDetail),
			Devices:    countFact(devicesInSubnet(devices, ipaddr, netmask)),
			Proto:      proto,
			Device:     device,
			PortDetail: portDetail,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// interfacePorts resolves a network's ports and their tagged/untagged membership:
// from the bridge-VLAN when the interface is VLAN-tagged, from the bridge's plain
// members otherwise, or the device itself when it is not a bridge.
func interfacePorts(cfg map[string]any, vlans []bridgeVLAN, bridge, id, device string) []widget.OverviewInterfacePort {
	if id != "" {
		for _, bv := range vlans {
			if bv.bridge == bridge && bv.id == id {
				out := make([]widget.OverviewInterfacePort, 0, len(bv.members))
				for _, m := range bv.members {
					out = append(out, widget.OverviewInterfacePort{Port: m.port, Mode: tagMode(m.tagged)})
				}
				return out
			}
		}
	}
	members := bridgePortsOf(cfg, bridge)
	if members == nil {
		members = []string{device} // not a bridge: the device is its own port
	}
	out := make([]widget.OverviewInterfacePort, 0, len(members))
	for _, p := range members {
		out = append(out, widget.OverviewInterfacePort{Port: p, Mode: "untagged"})
	}
	return out
}

// devicesInSubnet counts roster devices whose address sits in the network's
// subnet. A dynamic network (no static subnet) counts none — its membership is
// not derivable from config alone.
func devicesInSubnet(devices []widget.OverviewDevice, ipaddr, netmask string) int {
	cidr := subnetOf(ipaddr, netmask)
	if cidr == nil {
		return 0
	}
	n := 0
	for _, d := range devices {
		if ip := net.ParseIP(d.V4); ip != nil && cidr.Contains(ip) {
			n++
		}
	}
	return n
}

func tagMode(tagged bool) string {
	if tagged {
		return "tagged"
	}
	return "untagged"
}

// portSummary renders a network's port list for the row ("lan1, lan2, lan3").
func portSummary(ports []widget.OverviewInterfacePort) string {
	names := make([]string, 0, len(ports))
	for _, p := range ports {
		names = append(names, p.Port)
	}
	return strings.Join(names, ", ")
}

// isUplinkProto reports whether a protocol is a dynamic uplink worth a network
// row even with no static address.
func isUplinkProto(proto string) bool {
	switch proto {
	case "dhcp", "dhcpv6", "pppoe":
		return true
	}
	return false
}

// asSection narrows a uci config value to a section map.
func asSection(v any) map[string]any {
	s, _ := v.(map[string]any)
	return s
}

// uciScalar reads a uci option as a single string (a list yields its first).
func uciScalar(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if l := uciList(v); len(l) > 0 {
		return l[0]
	}
	return ""
}

// numLess orders VLAN id strings numerically ("2" before "10").
func numLess(a, b string) bool {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a < b
}

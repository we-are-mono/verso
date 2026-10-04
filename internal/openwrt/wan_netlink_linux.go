// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build linux

package openwrt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
)

const (
	rtaOIF       = 4
	rtaPriority  = 6
	rtaMultipath = 9
	rtaTable     = 15

	fraTable     = 15
	fraL3MDev    = 19
	frActToTable = 1

	iflaLinkInfo = 18
	iflaInfoKind = 1
	iflaInfoData = 2
	iflaVRFTable = 1

	rtnhDead     = 1
	rtnhLinkDown = 16
)

type netlinkAttr struct {
	typeID uint16
	value  []byte
}

// kernelDefaultRoutes dumps routes and rules without spawning ip(8). Only
// reachable, forwarding defaults survive; multipath routes yield every live
// output device.
func kernelDefaultRoutes() ([]kernelRoute, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	nameByIndex := make(map[int]string, len(interfaces))
	for _, iface := range interfaces {
		nameByIndex[iface.Index] = iface.Name
	}
	vrfTables, _ := kernelVRFTables()

	var out []kernelRoute
	for _, spec := range []struct {
		af, family int
	}{{syscall.AF_INET, 4}, {syscall.AF_INET6, 6}} {
		reachable, l3mdev := reachableRouteTables(spec.af)
		if l3mdev {
			for table := range vrfTables {
				reachable[table] = true
			}
		}
		data, err := syscall.NetlinkRIB(syscall.RTM_GETROUTE, spec.af)
		if err != nil {
			return nil, err
		}
		messages, err := syscall.ParseNetlinkMessage(data)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			if message.Header.Type != syscall.RTM_NEWROUTE || len(message.Data) < syscall.SizeofRtMsg {
				continue
			}
			// rtmsg: family, dst_len, src_len, tos, table, protocol,
			// scope, type, flags. A default has dst_len zero.
			if int(message.Data[0]) != spec.af || message.Data[1] != 0 || message.Data[7] != syscall.RTN_UNICAST {
				continue
			}
			flags := binary.NativeEndian.Uint32(message.Data[8:12])
			if flags&(rtnhDead|rtnhLinkDown) != 0 {
				continue
			}
			table := uint32(message.Data[4])
			attrs, attrErr := parseNetlinkAttrs(message.Data[syscall.SizeofRtMsg:])
			if attrErr != nil {
				continue
			}
			metric := uint32(0)
			var outputs []int
			for _, attr := range attrs {
				switch attr.typeID {
				case rtaTable:
					if value, ok := attrUint32(attr.value); ok {
						table = value
					}
				case rtaPriority:
					if value, ok := attrUint32(attr.value); ok {
						metric = value
					}
				case rtaOIF:
					if value, ok := attrUint32(attr.value); ok {
						outputs = append(outputs, int(value))
					}
				case rtaMultipath:
					outputs = append(outputs, multipathOutputs(attr.value)...)
				}
			}
			if table == 0 {
				table = mainRouteTable
			}
			if !reachable[table] {
				continue
			}
			seen := map[int]bool{}
			for _, index := range outputs {
				name := nameByIndex[index]
				if name == "" || seen[index] {
					continue
				}
				seen[index] = true
				out = append(out, kernelRoute{Family: spec.family, Device: name, Table: table, Metric: metric})
			}
		}
	}
	return out, nil
}

// reachableRouteTables interprets lookup rules. The main table is always a
// baseline even when a restricted kernel omits rule visibility.
func reachableRouteTables(family int) (map[uint32]bool, bool) {
	reachable := map[uint32]bool{mainRouteTable: true}
	data, err := syscall.NetlinkRIB(syscall.RTM_GETRULE, family)
	if err != nil {
		return reachable, false
	}
	messages, err := syscall.ParseNetlinkMessage(data)
	if err != nil {
		return reachable, false
	}
	l3mdev := false
	for _, message := range messages {
		if message.Header.Type != syscall.RTM_NEWRULE || len(message.Data) < syscall.SizeofRtMsg || message.Data[7] != frActToTable {
			continue
		}
		table := uint32(message.Data[4])
		attrs, err := parseNetlinkAttrs(message.Data[syscall.SizeofRtMsg:])
		if err != nil {
			continue
		}
		for _, attr := range attrs {
			switch attr.typeID {
			case fraTable:
				if value, ok := attrUint32(attr.value); ok {
					table = value
				}
			case fraL3MDev:
				if value, ok := attrUint32(attr.value); ok && value != 0 {
					l3mdev = true
				}
			}
		}
		if table != 0 {
			reachable[table] = true
		}
	}
	return reachable, l3mdev
}

// kernelVRFTables maps the l3mdev catch-all rule to the concrete tables owned
// by VRF link devices.
func kernelVRFTables() (map[uint32]bool, error) {
	out := map[uint32]bool{}
	data, err := syscall.NetlinkRIB(syscall.RTM_GETLINK, syscall.AF_UNSPEC)
	if err != nil {
		return out, err
	}
	messages, err := syscall.ParseNetlinkMessage(data)
	if err != nil {
		return out, err
	}
	for _, message := range messages {
		if message.Header.Type != syscall.RTM_NEWLINK || len(message.Data) < syscall.SizeofIfInfomsg {
			continue
		}
		attrs, err := parseNetlinkAttrs(message.Data[syscall.SizeofIfInfomsg:])
		if err != nil {
			continue
		}
		for _, attr := range attrs {
			if attr.typeID != iflaLinkInfo {
				continue
			}
			info, err := parseNetlinkAttrs(attr.value)
			if err != nil {
				continue
			}
			kind := ""
			var data []byte
			for _, nested := range info {
				switch nested.typeID {
				case iflaInfoKind:
					kind = string(bytes.TrimRight(nested.value, "\x00"))
				case iflaInfoData:
					data = nested.value
				}
			}
			if kind != "vrf" {
				continue
			}
			vrf, err := parseNetlinkAttrs(data)
			if err != nil {
				continue
			}
			for _, nested := range vrf {
				if nested.typeID == iflaVRFTable {
					if table, ok := attrUint32(nested.value); ok {
						out[table] = true
					}
				}
			}
		}
	}
	return out, nil
}

// liveRoutes is the kernel's main routing table, IPv4 then IPv6.
func liveRoutes() ([]liveRoute, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	nameByIndex := make(map[int]string, len(interfaces))
	for _, iface := range interfaces {
		nameByIndex[iface.Index] = iface.Name
	}
	out := []liveRoute{}
	for _, af := range []int{syscall.AF_INET, syscall.AF_INET6} {
		rib, err := syscall.NetlinkRIB(syscall.RTM_GETROUTE, af)
		if err != nil {
			return nil, err
		}
		out = append(out, liveRoutesFrom(rib, af, nameByIndex)...)
	}
	return out, nil
}

// routeProtocols names the protocols rtnetlink numbers (rtnetlink.h), as
// ip-route(8) prints them; any other prints as its number.
var routeProtocols = map[byte]string{1: "redirect", 2: "kernel", 3: "boot", 4: "static", 9: "ra", 16: "dhcp"}

// routeTypes are the route types a person states or reads in the main table:
// a unicast route, and the ones that refuse traffic. Local, broadcast,
// anycast and multicast entries are the kernel's own bookkeeping.
var routeTypes = map[byte]string{
	syscall.RTN_UNICAST: "", syscall.RTN_BLACKHOLE: "blackhole", syscall.RTN_UNREACHABLE: "unreachable",
	syscall.RTN_PROHIBIT: "prohibit", syscall.RTN_THROW: "throw",
}

// liveRoutesFrom reads one family's route dump as `ip route show table main`
// lists it. A cached clone (RTM_F_CLONED) is not a route anyone set.
func liveRoutesFrom(rib []byte, af int, nameByIndex map[int]string) []liveRoute {
	const (
		rtaDst, rtaGateway, rtaPrefSrc = 1, 5, 7
		rtmFCloned                     = 0x200
	)
	messages, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil
	}
	family := 4
	if af == syscall.AF_INET6 {
		family = 6
	}
	var out []liveRoute
	for _, message := range messages {
		data := message.Data
		if message.Header.Type != syscall.RTM_NEWROUTE || len(data) < syscall.SizeofRtMsg || int(data[0]) != af {
			continue
		}
		typ, kept := routeTypes[data[7]]
		if !kept || binary.NativeEndian.Uint32(data[8:12])&rtmFCloned != 0 {
			continue
		}
		attrs, err := parseNetlinkAttrs(data[syscall.SizeofRtMsg:])
		if err != nil {
			continue
		}
		table := uint32(data[4])
		dst := net.IPv4zero
		if family == 6 {
			dst = net.IPv6zero
		}
		route := liveRoute{Family: family, Type: typ, Proto: routeProtocols[data[5]]}
		if route.Proto == "" {
			route.Proto = fmt.Sprint(data[5])
		}
		for _, attr := range attrs {
			value, isUint := attrUint32(attr.value)
			switch attr.typeID {
			case rtaTable:
				if isUint {
					table = value
				}
			case rtaDst:
				dst = net.IP(attr.value)
			case rtaGateway:
				route.Gateway = net.IP(attr.value).String()
			case rtaPrefSrc:
				route.Source = net.IP(attr.value).String()
			case rtaPriority:
				route.Metric = value
			case rtaOIF:
				route.Device = nameByIndex[int(value)]
			case rtaMultipath:
				// ponytail: a multipath route reads as its first live output;
				// list every nexthop when ECMP is something Verso configures.
				if outputs := multipathOutputs(attr.value); len(outputs) > 0 {
					route.Device = nameByIndex[outputs[0]]
				}
			}
		}
		if table != mainRouteTable {
			continue
		}
		route.Target = fmt.Sprintf("%s/%d", dst, data[1])
		out = append(out, route)
	}
	return out
}

func multipathOutputs(data []byte) []int {
	var out []int
	for len(data) >= 8 {
		length := int(binary.NativeEndian.Uint16(data[:2]))
		if length < 8 || length > len(data) {
			break
		}
		flags := data[2]
		index := int(int32(binary.NativeEndian.Uint32(data[4:8])))
		if flags&(rtnhDead|rtnhLinkDown) == 0 && index > 0 {
			out = append(out, index)
		}
		aligned := align4(length)
		if aligned > len(data) {
			break
		}
		data = data[aligned:]
	}
	return out
}

func parseNetlinkAttrs(data []byte) ([]netlinkAttr, error) {
	var out []netlinkAttr
	for len(data) >= 4 {
		length := int(binary.NativeEndian.Uint16(data[:2]))
		if length < 4 || length > len(data) {
			return nil, fmt.Errorf("openwrt: malformed netlink attribute length %d", length)
		}
		typeID := binary.NativeEndian.Uint16(data[2:4]) & 0x3fff
		out = append(out, netlinkAttr{typeID: typeID, value: data[4:length]})
		aligned := align4(length)
		if aligned > len(data) {
			return nil, fmt.Errorf("openwrt: malformed aligned netlink attribute length %d", aligned)
		}
		data = data[aligned:]
	}
	return out, nil
}

func attrUint32(value []byte) (uint32, bool) {
	if len(value) < 4 {
		return 0, false
	}
	return binary.NativeEndian.Uint32(value[:4]), true
}

func align4(length int) int { return (length + 3) &^ 3 }

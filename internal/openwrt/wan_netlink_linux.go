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

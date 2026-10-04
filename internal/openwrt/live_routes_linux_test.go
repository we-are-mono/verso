// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build linux

package openwrt

import (
	"encoding/binary"
	"net"
	"reflect"
	"syscall"
	"testing"
)

// routeMessage spells one RTM_NEWROUTE as the kernel sends it in a dump.
func routeMessage(af, dstLen, table, proto, typ byte, flags uint32, attrs map[uint16][]byte) []byte {
	body := []byte{af, dstLen, 0, 0, table, proto, 0, typ, 0, 0, 0, 0}
	binary.NativeEndian.PutUint32(body[8:], flags)
	for _, id := range []uint16{1, 4, 5, 6, 7} {
		value, ok := attrs[id]
		if !ok {
			continue
		}
		attr := make([]byte, align4(4+len(value)))
		binary.NativeEndian.PutUint16(attr[0:], uint16(4+len(value)))
		binary.NativeEndian.PutUint16(attr[2:], id)
		copy(attr[4:], value)
		body = append(body, attr...)
	}
	header := make([]byte, syscall.NLMSG_HDRLEN)
	binary.NativeEndian.PutUint32(header[0:], uint32(syscall.NLMSG_HDRLEN+len(body)))
	binary.NativeEndian.PutUint16(header[4:], syscall.RTM_NEWROUTE)
	return append(header, body...)
}

func u32(n uint32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, n)
	return b
}

func TestLiveRoutesAreTheMainTableAsIpRouteShowsIt(t *testing.T) {
	names := map[int]string{2: "br-lan", 3: "eth1"}
	var rib []byte
	// default via 192.0.2.1 dev eth1 proto static metric 10
	rib = append(rib, routeMessage(syscall.AF_INET, 0, 254, 4, syscall.RTN_UNICAST, 0, map[uint16][]byte{
		4: u32(3), 5: net.ParseIP("192.0.2.1").To4(), 6: u32(10),
	})...)
	// 10.0.0.0/24 dev br-lan proto kernel scope link src 10.0.0.1
	rib = append(rib, routeMessage(syscall.AF_INET, 24, 254, 2, syscall.RTN_UNICAST, 0, map[uint16][]byte{
		1: net.ParseIP("10.0.0.0").To4(), 4: u32(2), 7: net.ParseIP("10.0.0.1").To4(),
	})...)
	// unreachable 10.99.0.0/16 proto static
	rib = append(rib, routeMessage(syscall.AF_INET, 16, 254, 4, syscall.RTN_UNREACHABLE, 0, map[uint16][]byte{
		1: net.ParseIP("10.99.0.0").To4(),
	})...)
	// local 10.0.0.1 dev br-lan table local: the router's own address, not a route anyone sets
	rib = append(rib, routeMessage(syscall.AF_INET, 32, 255, 2, syscall.RTN_LOCAL, 0, map[uint16][]byte{
		1: net.ParseIP("10.0.0.1").To4(), 4: u32(2),
	})...)
	// a route in another table belongs to policy routing, not this listing
	rib = append(rib, routeMessage(syscall.AF_INET, 8, 100, 4, syscall.RTN_UNICAST, 0, map[uint16][]byte{
		1: net.ParseIP("172.0.0.0").To4(), 4: u32(3),
	})...)
	// a cached clone is not a route either
	rib = append(rib, routeMessage(syscall.AF_INET, 32, 254, 2, syscall.RTN_UNICAST, 0x200, map[uint16][]byte{
		1: net.ParseIP("198.51.100.7").To4(), 4: u32(3),
	})...)

	got := liveRoutesFrom(rib, syscall.AF_INET, names)
	want := []liveRoute{
		{Family: 4, Target: "0.0.0.0/0", Gateway: "192.0.2.1", Device: "eth1", Metric: 10, Proto: "static"},
		{Family: 4, Target: "10.0.0.0/24", Device: "br-lan", Proto: "kernel", Source: "10.0.0.1"},
		{Family: 4, Target: "10.99.0.0/16", Proto: "static", Type: "unreachable"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("routes = %+v\nwant %+v", got, want)
	}
}

func TestLiveRoutesReadIPv6AndNameUnknownProtocolsByNumber(t *testing.T) {
	rib := routeMessage(syscall.AF_INET6, 64, 254, 9, syscall.RTN_UNICAST, 0, map[uint16][]byte{
		1: net.ParseIP("2001:db8:1::"), 4: u32(2), 5: net.ParseIP("fe80::1"), 6: u32(1024),
	})
	rib = append(rib, routeMessage(syscall.AF_INET6, 48, 254, 42, syscall.RTN_UNICAST, 0, map[uint16][]byte{
		1: net.ParseIP("2001:db8:2::"), 4: u32(2),
	})...)
	got := liveRoutesFrom(rib, syscall.AF_INET6, map[int]string{2: "br-lan"})
	want := []liveRoute{
		{Family: 6, Target: "2001:db8:1::/64", Gateway: "fe80::1", Device: "br-lan", Metric: 1024, Proto: "ra"},
		{Family: 6, Target: "2001:db8:2::/48", Device: "br-lan", Proto: "42"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("routes = %+v\nwant %+v", got, want)
	}
}

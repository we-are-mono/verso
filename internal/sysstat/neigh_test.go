// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

import (
	"encoding/binary"
	"syscall"
	"testing"
)

// attr appends one 4-aligned netlink attribute.
func attr(data []byte, atype uint16, payload []byte) []byte {
	alen := 4 + len(payload)
	buf := make([]byte, (alen+3)&^3)
	binary.NativeEndian.PutUint16(buf[0:2], uint16(alen))
	binary.NativeEndian.PutUint16(buf[2:4], atype)
	copy(buf[4:], payload)
	return append(data, buf...)
}

// neighMsg builds one RTM_NEWNEIGH wire message: the 12-byte ndmsg with the
// state at offset 8, then NDA_DST and (optionally) NDA_LLADDR attributes.
func neighMsg(ifindex int, state uint16, dst []byte, mac []byte) syscall.NetlinkMessage {
	data := make([]byte, ndmsgLen)
	data[0] = syscall.AF_INET
	binary.NativeEndian.PutUint32(data[4:8], uint32(ifindex))
	binary.NativeEndian.PutUint16(data[8:10], state)
	data = attr(data, ndaDST, dst)
	if mac != nil {
		data = attr(data, ndaLLAddr, mac)
	}
	return syscall.NetlinkMessage{
		Header: syscall.NlMsghdr{Type: syscall.RTM_NEWNEIGH},
		Data:   data,
	}
}

// TestParseNeighbors: v4 and v6 destinations land with their states and
// MACs; non-neighbour messages (the dump's trailing NLMSG_DONE) are skipped.
func TestParseNeighbors(t *testing.T) {
	v6 := make([]byte, 16)
	v6[0], v6[1], v6[15] = 0xfd, 0x42, 0x99
	neigh := parseNeighborsWithInterfaceName([]syscall.NetlinkMessage{
		neighMsg(7, 0x02, []byte{192, 168, 77, 138}, []byte{0xa2, 0xba, 0xa6, 0x35, 0x2b, 0xb8}), // reachable
		neighMsg(7, 0x04, []byte{192, 168, 77, 120}, nil),                                        // stale, unresolved
		neighMsg(7, 0x02, v6, []byte{0xa2, 0xba, 0xa6, 0x35, 0x2b, 0xb8}),
		{Header: syscall.NlMsghdr{Type: syscall.NLMSG_DONE}},
	}, func(index int) string {
		if index == 7 {
			return "br-lan.10"
		}
		return ""
	})
	if len(neigh) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(neigh), neigh)
	}
	first := neigh[0]
	if first.Addr != "192.168.77.138" || first.MAC != "a2:ba:a6:35:2b:b8" ||
		first.Interface != "br-lan.10" || !first.State.Active() || first.State.Recent() {
		t.Errorf("reachable v4 = %+v", first)
	}
	second := neigh[1]
	if second.Addr != "192.168.77.120" || second.MAC != "" || second.State.Active() || !second.State.Recent() {
		t.Errorf("stale v4 = %+v", second)
	}
	third := neigh[2]
	if third.Addr != "fd42::99" || third.MAC != "a2:ba:a6:35:2b:b8" {
		t.Errorf("v6 entry = %+v", third)
	}
}

// TestNeighStateNames: detail views print the state the way `ip neigh` does.
func TestNeighStateNames(t *testing.T) {
	cases := map[uint16]string{0x02: "reachable", 0x04: "stale", 0x80: "permanent", 0x00: "none"}
	for raw, want := range cases {
		if got := NeighState(raw).String(); got != want {
			t.Errorf("state %#x = %q, want %q", raw, got, want)
		}
	}
}

// TestNeighborsLive: the real dual-family dump runs (linux-only code,
// linux-only tests) and returns without error — content depends on the box.
func TestNeighborsLive(t *testing.T) {
	if _, err := Neighbors(); err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
}

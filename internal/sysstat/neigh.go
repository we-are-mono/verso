// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// NeighState is the kernel's confidence in one neighbour-table entry — the
// NUD state from rtnetlink. /proc/net/arp cannot say this: its flags only
// distinguish complete from incomplete, and a complete entry lingers long
// after the device left.
type NeighState uint16

const (
	nudReachable NeighState = 0x02
	nudStale     NeighState = 0x04
	nudDelay     NeighState = 0x08
	nudProbe     NeighState = 0x10
	nudPermanent NeighState = 0x80
)

// Active: the kernel has current confirmation — the neighbour answered
// recently (reachable), is being re-confirmed right now (delay, probe), or is
// pinned (permanent).
func (s NeighState) Active() bool {
	return s&(nudReachable|nudDelay|nudProbe|nudPermanent) != 0
}

// Recent: the entry stands but confirmation has lapsed — seen lately, quiet
// since.
func (s NeighState) Recent() bool { return s&nudStale != 0 }

// String names the state the way `ip neigh` does, for detail views.
func (s NeighState) String() string {
	switch {
	case s&nudReachable != 0:
		return "reachable"
	case s&nudStale != 0:
		return "stale"
	case s&nudDelay != 0:
		return "delay"
	case s&nudProbe != 0:
		return "probe"
	case s&nudPermanent != 0:
		return "permanent"
	}
	return "none"
}

// Neighbor is one neighbour-table entry: the address, the MAC it resolved to
// (empty while unresolved), and the kernel's confidence. The MAC is the join
// key that groups a device's v4 and v6 addresses into one identity.
type Neighbor struct {
	Addr  string
	MAC   string
	State NeighState
}

// Neighbors dumps the kernel's neighbour table over rtnetlink — both
// families, all interfaces.
func Neighbors() ([]Neighbor, error) {
	var all []Neighbor
	for _, family := range []int{syscall.AF_INET, syscall.AF_INET6} {
		dump, err := syscall.NetlinkRIB(syscall.RTM_GETNEIGH, family)
		if err != nil {
			return nil, fmt.Errorf("sysstat: neighbour dump: %w", err)
		}
		msgs, err := syscall.ParseNetlinkMessage(dump)
		if err != nil {
			return nil, fmt.Errorf("sysstat: neighbour dump: %w", err)
		}
		all = append(all, parseNeighbors(msgs)...)
	}
	return all, nil
}

// BridgePorts dumps the bridge forwarding database (the AF_BRIDGE neighbour
// family): MAC → the interface it was learned on — the join that puts a
// device on a physical port.
func BridgePorts() (map[string]string, error) {
	msgs, err := fdbDump()
	if err != nil {
		return nil, fmt.Errorf("sysstat: fdb dump: %w", err)
	}
	ports := make(map[string]string)
	for _, m := range msgs {
		if m.Header.Type != syscall.RTM_NEWNEIGH || len(m.Data) < ndmsgLen {
			continue
		}
		ifindex := int(int32(binary.NativeEndian.Uint32(m.Data[4:8])))
		mac := ""
		for rest := m.Data[ndmsgLen:]; len(rest) >= 4; {
			alen := int(binary.NativeEndian.Uint16(rest[0:2]))
			if alen < 4 || alen > len(rest) {
				break
			}
			if binary.NativeEndian.Uint16(rest[2:4]) == ndaLLAddr && alen == 10 {
				mac = net.HardwareAddr(rest[4:10]).String()
			}
			rest = rest[(alen+3)&^3:]
		}
		if mac == "" {
			continue
		}
		if ifi, err := net.InterfaceByIndex(ifindex); err == nil {
			ports[mac] = ifi.Name
		}
	}
	return ports, nil
}

// fdbDump issues the RTM_GETNEIGH dump with an ifinfomsg body and
// family AF_BRIDGE — the shape the bridge module expects for
// forwarding-database dumps. (NetlinkRIB sends a plain rtgenmsg, which the
// kernel's strict checking answers with an empty dump here.)
func fdbDump() ([]syscall.NetlinkMessage, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)
	lsa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Bind(fd, lsa); err != nil {
		return nil, err
	}
	req := make([]byte, 32) // nlmsghdr (16) + ifinfomsg (16)
	binary.NativeEndian.PutUint32(req[0:4], 32)
	binary.NativeEndian.PutUint16(req[4:6], uint16(syscall.RTM_GETNEIGH))
	binary.NativeEndian.PutUint16(req[6:8], syscall.NLM_F_DUMP|syscall.NLM_F_REQUEST)
	binary.NativeEndian.PutUint32(req[8:12], 1) // sequence
	req[16] = syscall.AF_BRIDGE                 // ifinfomsg.ifi_family
	if err := syscall.Sendto(fd, req, 0, lsa); err != nil {
		return nil, err
	}
	var msgs []syscall.NetlinkMessage
	buf := make([]byte, 1<<16)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, err
		}
		batch, err := syscall.ParseNetlinkMessage(append([]byte(nil), buf[:n]...))
		if err != nil {
			return nil, err
		}
		for _, m := range batch {
			switch m.Header.Type {
			case syscall.NLMSG_DONE:
				return msgs, nil
			case syscall.NLMSG_ERROR:
				return nil, syscall.EINVAL
			default:
				msgs = append(msgs, m)
			}
		}
	}
}

// The wire shapes (linux uapi): each RTM_NEWNEIGH message carries a 12-byte
// struct ndmsg — family, padding, ifindex, then the u16 state at offset 8 —
// followed by 4-aligned netlink attributes; NDA_DST (type 1) is the address
// (4 or 16 bytes), NDA_LLADDR (type 2) the link-layer address.
const (
	ndmsgLen  = 12
	ndaDST    = 1
	ndaLLAddr = 2
)

func parseNeighbors(msgs []syscall.NetlinkMessage) []Neighbor {
	var neigh []Neighbor
	for _, m := range msgs {
		if m.Header.Type != syscall.RTM_NEWNEIGH || len(m.Data) < ndmsgLen {
			continue
		}
		n := Neighbor{State: NeighState(binary.NativeEndian.Uint16(m.Data[8:10]))}
		for rest := m.Data[ndmsgLen:]; len(rest) >= 4; {
			alen := int(binary.NativeEndian.Uint16(rest[0:2]))
			atype := binary.NativeEndian.Uint16(rest[2:4])
			if alen < 4 || alen > len(rest) {
				break
			}
			switch {
			case atype == ndaDST && alen == 8:
				n.Addr = netip.AddrFrom4([4]byte(rest[4:8])).String()
			case atype == ndaDST && alen == 20:
				n.Addr = netip.AddrFrom16([16]byte(rest[4:20])).String()
			case atype == ndaLLAddr && alen == 10:
				n.MAC = net.HardwareAddr(rest[4:10]).String()
			}
			rest = rest[(alen+3)&^3:]
		}
		if n.Addr != "" {
			neigh = append(neigh, n)
		}
	}
	return neigh
}

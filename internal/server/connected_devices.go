// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

// connectedDevices builds the overview's Connected-devices roster — every device
// the box has seen, not just DHCP leaseholders. The kernel neighbour table is the
// seed (each MAC with the addresses tied to it, both families), and the DHCP lease
// enriches a device with its friendly name and expiry when there is one. So a
// static host, a v6-only SLAAC device, and a DHCP client all appear alike, keyed
// by MAC — the identity a person recognises, not the DHCPv6 DUID.
func (s *Server) connectedDevices(ctx context.Context, sid string) []widget.OverviewDevice {
	neigh, err := s.neighbors()
	if err != nil {
		log.Printf("verso: devices: neighbour table unavailable: %v", err)
	}
	agg := aggregateNeighbors(neigh)

	leases := map[string]lease{}
	if raw, err := s.readLeases(); err != nil {
		log.Printf("verso: devices: leases unavailable: %v", err)
	} else {
		for _, l := range parseLeases(raw) {
			leases[strings.ToLower(l.mac)] = l
		}
	}

	ports, err := s.bridgePorts()
	if err != nil {
		log.Printf("verso: devices: fdb unavailable: %v", err)
	}
	zones := s.zoneMap(ctx, sid)

	// The interface a device sits on — the finer segment label the row shows,
	// derived from its address the same way the zone is.
	var nets []ifaceNet
	if cfg, err := s.backend.UCIConfig(ctx, sid, "network"); err == nil {
		nets = interfaceNets(cfg)
	} else {
		log.Printf("verso: devices: network config unavailable: %v", err)
	}

	// DHCPv6 DUIDs, keyed by an assigned address — DHCPv6 carries no MAC, so a
	// lease joins to a device through a v6 address they share.
	duidByAddr := map[string]string{}
	if v6, err := s.backend.IPv6Leases(ctx, sid); err != nil {
		log.Printf("verso: devices: ipv6 leases unavailable: %v", err)
	} else {
		for _, l := range v6 {
			for _, a := range l.Addrs {
				duidByAddr[a] = l.DUID
			}
		}
	}

	byAddr := map[string]sysstat.DeviceTraffic{}
	if bt, err := s.backend.ConnStats(ctx, sid); err != nil {
		log.Printf("verso: devices: conntrack unavailable: %v", err)
	} else {
		byAddr = bt
	}

	// The device set is the union of every MAC the kernel knows and every MAC
	// holding a lease — so nothing that talks to the box is missed.
	macs := make(map[string]bool, len(agg)+len(leases))
	for m := range agg {
		macs[m] = true
	}
	for m := range leases {
		macs[m] = true
	}

	now := time.Now()
	out := make([]widget.OverviewDevice, 0, len(macs))
	for mac := range macs {
		entries := agg[mac]
		l, hasLease := leases[mac]

		v4, v6, all := deviceAddresses(l, hasLease, entries)
		zoneAddr := v4
		if zoneAddr == "" {
			zoneAddr = v6
		}
		host := ""
		if hasLease {
			host = l.host
		}
		tr := deviceTrafficTotal(all, byAddr)

		out = append(out, widget.OverviewDevice{
			Name:       deviceName(host, mac),
			MAC:        mac,
			DUID:       duidFor(all, duidByAddr),
			V4:         v4,
			V6:         v6,
			Interface:  ifaceForAddr(nets, zoneAddr),
			Zone:       zoneForAddr(zones, zoneAddr),
			Presence:   presenceWord(bestPresence(entries)),
			Addresses:  all,
			Connection: ports[mac],
			Lease:      leaseFact(l, hasLease, now),
			Traffic:    trafficFact(tr),
			Conns:      countFact(tr.Conns),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if pi, pj := presenceRank(out[i].Presence), presenceRank(out[j].Presence); pi != pj {
			return pi > pj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// deviceAddresses returns the device's primary v4 and primary global v6 (for the
// stacked cell) and the full, de-duplicated address list (for the drawer). The
// lease's v4 leads; neighbour entries follow in their ranked order (globals
// before link-locals), so the primary v6 is the first non-link-local address.
func deviceAddresses(l lease, hasLease bool, entries []sysstat.Neighbor) (v4, v6 string, all []widget.OverviewAddr) {
	seen := map[string]bool{}
	add := func(addr, state string) {
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		fam := "IPv4"
		if strings.Contains(addr, ":") {
			fam = "IPv6"
		}
		all = append(all, widget.OverviewAddr{Addr: addr, Family: fam, State: state})
		switch {
		case fam == "IPv4":
			if v4 == "" {
				v4 = addr
			}
		case !strings.HasPrefix(addr, "fe80"):
			if v6 == "" {
				v6 = addr
			}
		}
	}
	if hasLease {
		add(l.ip, neighborState(entries, l.ip))
	}
	for _, n := range entries {
		add(n.Addr, neighWord(n.State))
	}
	return v4, v6, all
}

// duidFor returns a device's DHCPv6 DUID — the DUID of the first of its addresses
// a DHCPv6 lease was issued against, or "" when it holds no v6 lease.
func duidFor(addrs []widget.OverviewAddr, duidByAddr map[string]string) string {
	for _, a := range addrs {
		if d, ok := duidByAddr[a.Addr]; ok {
			return d
		}
	}
	return ""
}

// neighborState finds a specific address's confidence word among the entries, or
// "" when the kernel has no entry for it (a lease address never yet contacted).
func neighborState(entries []sysstat.Neighbor, addr string) string {
	for _, n := range entries {
		if n.Addr == addr {
			return neighWord(n.State)
		}
	}
	return ""
}

// neighWord renders a neighbour's NUD confidence in plain language.
func neighWord(st sysstat.NeighState) string {
	switch {
	case st.Active():
		return "reachable"
	case st.Recent():
		return "stale"
	default:
		return "offline"
	}
}

// presenceWord / presenceRank map the roster's tri-state presence to the widget
// vocabulary and back to a sort key (online first).
func presenceWord(p presence) string {
	switch p {
	case presenceOnline:
		return "online"
	case presenceIdle:
		return "idle"
	default:
		return "offline"
	}
}

func presenceRank(word string) int {
	switch word {
	case "online":
		return 2
	case "idle":
		return 1
	default:
		return 0
	}
}

// deviceTrafficTotal sums a device's conntrack byte counters across every address
// it answers to.
func deviceTrafficTotal(addrs []widget.OverviewAddr, byAddr map[string]sysstat.DeviceTraffic) sysstat.DeviceTraffic {
	var t sysstat.DeviceTraffic
	for _, a := range addrs {
		if at, ok := byAddr[a.Addr]; ok {
			t.RxBytes += at.RxBytes
			t.TxBytes += at.TxBytes
			t.Conns += at.Conns
		}
	}
	return t
}

// leaseFact renders the DHCP-lease line for the drawer: the time remaining, or a
// plain statement for a device with no lease at all.
func leaseFact(l lease, hasLease bool, now time.Time) string {
	if !hasLease {
		return "No DHCP lease"
	}
	return leaseIn(l.expiry, now)
}

// trafficFact renders a device's totals as "12.4 GB down · 3.1 GB up"; a device
// with no observed traffic yields "" so its row shows a dash.
func trafficFact(t sysstat.DeviceTraffic) string {
	if t.RxBytes == 0 && t.TxBytes == 0 {
		return ""
	}
	return gb(t.RxBytes) + " GB down · " + gb(t.TxBytes) + " GB up"
}

// countFact renders a non-zero count, or "" so the row reads as a dash.
func countFact(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

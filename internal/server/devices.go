// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
)

// Connected devices: the roster, from the router's own truth. dnsmasq's
// lease file names who holds an address; the kernel supplies everything
// else, joined by MAC — neighbour entries both families (with real NUD
// confidence) and the bridge port the MAC was learned on. Friendly names lead;
// each row opens the device's full story in a wide drawer.

const leasesPath = "/tmp/dhcp.leases"

// presence is the roster's honest tri-state: the kernel confirmed the device
// recently (online), holds a lapsed entry (idle — seen lately, quiet since),
// or has nothing current (offline).
type presence int

const (
	presenceOffline presence = iota
	presenceIdle
	presenceOnline
)

// aggregateNeighbors folds the neighbour table per MAC (lowercased): every
// entry, v4 first, then v6 globals, link-locals last. An entry without a MAC
// (unresolved, failed) says nothing about any device.
func aggregateNeighbors(neigh []sysstat.Neighbor) map[string][]sysstat.Neighbor {
	agg := make(map[string][]sysstat.Neighbor)
	for _, n := range neigh {
		if n.MAC == "" {
			continue
		}
		mac := strings.ToLower(n.MAC)
		agg[mac] = append(agg[mac], n)
	}
	rank := func(addr string) int {
		switch {
		case !strings.Contains(addr, ":"):
			return 0
		case !strings.HasPrefix(addr, "fe80"):
			return 1
		}
		return 2
	}
	for _, entries := range agg {
		sort.Slice(entries, func(i, j int) bool {
			if ri, rj := rank(entries[i].Addr), rank(entries[j].Addr); ri != rj {
				return ri < rj
			}
			return entries[i].Addr < entries[j].Addr
		})
	}
	return agg
}

// bestPresence is the strongest confidence across a device's addresses.
func bestPresence(entries []sysstat.Neighbor) presence {
	best := presenceOffline
	for _, n := range entries {
		switch {
		case n.State.Active():
			return presenceOnline
		case n.State.Recent():
			best = presenceIdle
		}
	}
	return best
}

// leaseIn says when the lease runs out the way a person would.
func leaseIn(expiry int64, now time.Time) string {
	d := time.Unix(expiry, 0).Sub(now)
	switch {
	case d <= 0:
		return "expired"
	case d < time.Hour:
		return fmt.Sprintf("in %d min", int(d.Minutes()))
	}
	return fmt.Sprintf("in %dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// lease is one dnsmasq lease-file line: expiry, MAC, address, hostname,
// client id — hostname is "*" when the device offered none.
type lease struct {
	expiry        int64
	mac, ip, host string
}

func parseLeases(raw []byte) []lease {
	var leases []lease
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		expiry, _ := strconv.ParseInt(fields[0], 10, 64)
		leases = append(leases, lease{expiry: expiry, mac: fields[1], ip: fields[2], host: fields[3]})
	}
	return leases
}

// deviceName prefers the DHCP hostname; a device that offered none gets a
// quiet stand-in from its MAC tail ("Device b7:af") rather than a full MAC.
func deviceName(host, mac string) string {
	if host != "" && host != "*" {
		return host
	}
	if parts := strings.Split(mac, ":"); len(parts) >= 2 {
		return "Device " + strings.Join(parts[len(parts)-2:], ":")
	}
	return "Device"
}

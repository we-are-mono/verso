// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
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

// deviceEntry is one device: the lease identity, enriched with everything
// the kernel ties to its MAC.
type deviceEntry struct {
	Name        string
	IP          string
	MAC         string
	Icon        string
	Presence    presence
	Zone        string             // firewall zone its subnet sits behind
	Port        string             // bridge port the MAC was learned on ("" unknown)
	LeaseExpiry int64              // unix; when the DHCP lease runs out
	Addrs       []sysstat.Neighbor // every address the kernel ties to the MAC, states included
	Traffic     sysstat.DeviceTraffic
	Down, Up    []float64 // last-minute rate history in Mbps (traffic_history.go)
}

// ipv6 lists the device's v6 addresses (already ordered globals-first).
func (d deviceEntry) ipv6() []string {
	var out []string
	for _, a := range d.Addrs {
		if strings.Contains(a.Addr, ":") {
			out = append(out, a.Addr)
		}
	}
	return out
}

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

// deviceList reads the roster. No lease file (dnsmasq not serving) means no
// roster — the page then carries no section; every other source degrades to
// its own absence.
func (s *Server) deviceList(ctx context.Context, sid string) []deviceEntry {
	raw, err := s.readLeases()
	if err != nil {
		log.Printf("verso: devices: leases unavailable: %v", err)
		return nil
	}
	leases := parseLeases(raw)

	neigh, err := s.neighbors()
	if err != nil {
		log.Printf("verso: devices: neighbour table unavailable: %v", err)
	}
	agg := aggregateNeighbors(neigh)

	ports, err := s.bridgePorts()
	if err != nil {
		log.Printf("verso: devices: fdb unavailable: %v", err)
	}
	zones := s.zoneMap(ctx, sid)

	var devices []deviceEntry
	for _, l := range leases {
		mac := strings.ToLower(l.mac)
		entries := agg[mac]
		devices = append(devices, deviceEntry{
			Name:        deviceName(l.host, l.mac),
			IP:          l.ip,
			MAC:         mac,
			Icon:        deviceIcon(l.host),
			Presence:    bestPresence(entries),
			Zone:        zoneForAddr(zones, l.ip),
			Port:        ports[mac],
			LeaseExpiry: l.expiry,
			Addrs:       entries,
			Down:        []float64{0, 0},
			Up:          []float64{0, 0},
		})
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Presence != devices[j].Presence {
			return devices[i].Presence > devices[j].Presence
		}
		return devices[i].Name < devices[j].Name
	})
	return devices
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

// deviceIcon guesses a silhouette from the hostname — a hint, not a claim;
// anything unrecognised is the generic device.
func deviceIcon(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "phone"), strings.Contains(h, "android"),
		strings.Contains(h, "pixel"), strings.Contains(h, "galaxy"),
		strings.Contains(h, "ipad"):
		return "phone"
	case strings.Contains(h, "tv"), strings.Contains(h, "roku"),
		strings.Contains(h, "chromecast"), strings.Contains(h, "shield"):
		return "tv"
	case strings.Contains(h, "book"), strings.Contains(h, "laptop"),
		strings.Contains(h, "desktop"), strings.Contains(h, "pc"):
		return "laptop"
	case strings.Contains(h, "router"), strings.Contains(h, "switch"),
		strings.Contains(h, "ap-"):
		return "router"
	}
	return "device"
}

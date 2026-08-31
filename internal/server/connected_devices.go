// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/deviceicon"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

// connectedDevices builds the Devices page's roster — every device the box has
// seen, not just DHCP leaseholders. The kernel neighbour table is the seed (each
// MAC with the addresses tied to it, both families), and the DHCP lease enriches
// a device with its friendly name and expiry when there is one. So a static host,
// a v6-only SLAAC device, and a DHCP client all appear alike, keyed by MAC — the
// identity a person recognises, not the DHCPv6 DUID.
func (s *Server) connectedDevices(ctx context.Context, sid string) []widget.Device {
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
	// The interface a device sits on — the finer segment label the row shows,
	// derived from its address the same way the zone is.
	var nets []ifaceNet
	var zones []zoneNet
	zoneOfDevice := map[string]string{}
	netCfg, netErr := s.backend.UCIConfig(ctx, sid, "network")
	fwCfg, fwErr := s.backend.UCIConfig(ctx, sid, "firewall")
	if netErr == nil {
		nets = interfaceNets(netCfg)
	} else {
		log.Printf("verso: devices: network config unavailable: %v", netErr)
	}
	if fwErr != nil {
		log.Printf("verso: devices: firewall config unavailable: %v", fwErr)
	}
	if netErr == nil && fwErr == nil {
		zones = zoneNets(netCfg, fwCfg)
		zoneOfDevice = zonesByDevice(netCfg, fwCfg)
	}

	// Reservations are config, not runtime: a `host` section in the dhcp config
	// pins one MAC to one address, which is what tells a device holding a lease
	// apart from one that will keep the address it holds.
	reserved := map[string]bool{}
	if dhcpCfg, err := s.backend.UCIConfig(ctx, sid, "dhcp"); err != nil {
		log.Printf("verso: devices: dhcp config unavailable: %v", err)
	} else {
		reserved = reservedMACs(dhcpCfg)
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
	// Resolved once: the liveness check behind it is a socket connect, and every
	// row would otherwise repeat it.
	reserveDoor := s.reserveDoor()
	out := make([]widget.Device, 0, len(macs))
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
		iface := neighborInterface(entries, zoneAddr)
		if iface == "" {
			iface = ifaceForAddr(nets, zoneAddr)
		}
		zone := zoneForAddr(zones, zoneAddr)
		if zone == "" {
			zone = zoneOfDevice[iface]
		}
		out = append(out, widget.Device{
			Name:        deviceName(host, mac),
			Icon:        deviceicon.Resolve(mac, host),
			MAC:         mac,
			DUID:        duidFor(all, duidByAddr),
			V4:          v4,
			V6:          v6,
			Interface:   iface,
			Zone:        zone,
			Presence:    presenceWord(bestPresence(entries)),
			Addresses:   all,
			Connection:  ports[mac],
			Lease:       leaseFact(l, hasLease, now),
			Leased:      hasLease,
			Reserved:    reserved[mac],
			ReserveHref: reserveDoor(mac),
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

// neighborInterface returns the kernel interface that owns a device's primary
// address. This is stronger than inferring it from UCI and also works for
// delegated IPv6 prefixes; another known address is the fallback.
func neighborInterface(entries []sysstat.Neighbor, primaryAddr string) string {
	for _, entry := range entries {
		if entry.Addr == primaryAddr && entry.Interface != "" {
			return entry.Interface
		}
	}
	for _, entry := range entries {
		if entry.Interface != "" {
			return entry.Interface
		}
	}
	return ""
}

// deviceAddresses returns the device's primary v4 and primary global v6 (for the
// stacked cell) and the full, de-duplicated address list (for the drawer). The
// lease's v4 leads; neighbour entries follow in their ranked order (globals
// before link-locals), so the primary v6 is the first non-link-local address.
func deviceAddresses(l lease, hasLease bool, entries []sysstat.Neighbor) (v4, v6 string, all []widget.DeviceAddr) {
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
		all = append(all, widget.DeviceAddr{Addr: addr, Family: fam, State: state})
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
func duidFor(addrs []widget.DeviceAddr, duidByAddr map[string]string) string {
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

// leaseFact renders the DHCP-lease line for the drawer: the time remaining, or a
// plain statement for a device with no lease at all.
func leaseFact(l lease, hasLease bool, now time.Time) string {
	if !hasLease {
		return "No DHCP lease"
	}
	return leaseIn(l.expiry, now)
}

// reservedMACs is every MAC the dhcp config pins to an address — a `host`
// section's mac option, lowercased to join with the roster's keys. A section may
// name several MACs for one reservation, so each is taken on its own.
func reservedMACs(cfg map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, v := range cfg {
		s := asSection(v)
		if s == nil || s[".type"] != "host" {
			continue
		}
		switch mac := s["mac"].(type) {
		case string:
			for _, one := range strings.Fields(mac) {
				out[strings.ToLower(one)] = true
			}
		case []any:
			for _, entry := range mac {
				if one, ok := entry.(string); ok {
					out[strings.ToLower(one)] = true
				}
			}
		}
	}
	return out
}

// dnsdhcpPluginID owns reservations: the DHCP page is where an address a device
// holds becomes an address it keeps.
const dnsdhcpPluginID = "dnsdhcp"

// reserveDoor returns the link builder for a device's reservation form, or one
// that leads nowhere while no plugin serves that page — a door with nothing
// behind it is worse than none. The MAC rides as a query parameter the Leases
// page reads to open that device's panel.
func (s *Server) reserveDoor() func(mac string) string {
	m, ok := s.manifestByID(dnsdhcpPluginID)
	if !ok || !s.probe(m.Socket) {
		return func(string) string { return "" }
	}
	return func(mac string) string {
		return pluginHref(dnsdhcpPluginID, "/") + "?reserve=" + url.QueryEscape(mac)
	}
}

// onlineDevices counts the devices on the network right now: everything the
// kernel still holds a live neighbour mapping for. Strict reachability is the
// wrong bar — a connected device's entry decays to stale within seconds of not
// talking to the router itself, and bridge traffic never refreshes it, so a
// reachable-only count reads one or two on a busy network. The neighbour dump
// alone answers it — no leases, no config, no helper call — so the sidebar can
// carry the number on every page. An unreadable table reports no count rather
// than a zero, since "nobody is here" is a different statement.
func (s *Server) onlineDevices() (int, bool) {
	neigh, err := s.neighbors()
	if err != nil {
		log.Printf("verso: devices: neighbour table unavailable: %v", err)
		return 0, false
	}
	online := 0
	for _, entries := range aggregateNeighbors(neigh) {
		if bestPresence(entries) != presenceOffline {
			online++
		}
	}
	return online, true
}

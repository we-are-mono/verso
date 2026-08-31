// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// The brokered `dhcpLeases` read: who holds an address on this router right now.
// Config says which addresses may be handed out; only the live lease table says
// which ones are. A plugin cannot read either source itself (ADR-007), so the
// shell reads both with the operator's session and hands down one list.
//
// dnsmasq's lease file is the spine — it is the DHCPv4 table, and it is what the
// DHCP domain is about — and odhcpd's DHCPv6 table enriches a device that also
// holds v6 addresses. A DHCPv6 lease that joins no v4 lease is left out: it
// carries no MAC, which is the identity everything downstream (a reservation, a
// device's name) is keyed by.

// dhcpLease is one device's live lease, as the plugin receives it. Expiry is the
// lease file's absolute epoch second, so the plugin renders the countdown
// against its own clock rather than trusting a duration computed here.
type dhcpLease struct {
	Hostname  string   `json:"hostname"`
	MAC       string   `json:"mac"`
	IPv4      string   `json:"ipv4"`
	IPv6s     []string `json:"ipv6s"`
	ExpiresAt int64    `json:"expires_at"`
}

// dhcpLeases answers the brokered read. A failure to read the v6 table costs
// only the v6 addresses; a failure to read the lease file is the whole answer,
// and the gateway degrades the page rather than failing it.
func (s *Server) dhcpLeases(ctx context.Context, sid string) (json.RawMessage, error) {
	raw, err := s.readLeases()
	if err != nil {
		return nil, err
	}

	var v6 []openwrt.V6Lease
	if v6, err = s.backend.IPv6Leases(ctx, sid); err != nil {
		log.Printf("verso: dhcp leases: ipv6 leases unavailable: %v", err)
	}

	leases := make([]dhcpLease, 0)
	for _, l := range parseLeases(raw) {
		leases = append(leases, dhcpLease{
			Hostname:  leaseHostname(l.host),
			MAC:       strings.ToLower(l.mac),
			IPv4:      l.ip,
			IPv6s:     v6AddrsFor(v6, l),
			ExpiresAt: l.expiry,
		})
	}
	// The lease file's order is the order addresses happened to be handed out;
	// address order groups a network's devices together, which is how the leases
	// are read.
	sort.Slice(leases, func(i, j int) bool {
		if a, b := ipv4Key(leases[i].IPv4), ipv4Key(leases[j].IPv4); a != b {
			return a < b
		}
		return leases[i].MAC < leases[j].MAC
	})
	return json.Marshal(map[string]any{"leases": leases})
}

// leaseHostname drops dnsmasq's stand-in for a device that offered no name —
// naming it is the plugin's to do, and "*" is not a name.
func leaseHostname(host string) string {
	if host == "*" {
		return ""
	}
	return host
}

// v6AddrsFor is the device's DHCPv6 addresses. DHCPv6 carries no MAC, so the
// join is the DUID's embedded link-layer address (DUID-LLT and DUID-LL both end
// in it) and, where the DUID carries none, the hostname both tables recorded.
func v6AddrsFor(v6 []openwrt.V6Lease, l lease) []string {
	var out []string
	mac := strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(l.mac))
	host := leaseHostname(l.host)
	for _, entry := range v6 {
		byMAC := mac != "" && strings.HasSuffix(strings.ToLower(entry.DUID), mac)
		byHost := host != "" && strings.EqualFold(entry.Hostname, host)
		if byMAC || byHost {
			out = append(out, entry.Addrs...)
		}
	}
	return out
}

// ipv4Key orders dotted-quad addresses the way they read, not the way they
// sort as text. An address that is not one sorts last.
func ipv4Key(addr string) uint64 {
	octets := strings.Split(addr, ".")
	if len(octets) != 4 {
		return 1 << 32
	}
	var key uint64
	for _, octet := range octets {
		n := 0
		for _, c := range octet {
			if c < '0' || c > '9' {
				return 1 << 32
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return 1 << 32
		}
		key = key<<8 | uint64(n)
	}
	return key
}

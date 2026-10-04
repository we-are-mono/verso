// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

// The Devices page: the roster, from the router's own truth. dnsmasq's
// lease file names who holds an address; the kernel supplies everything
// else, joined by MAC — neighbour entries both families (with real NUD
// confidence) and the bridge port the MAC was learned on. Friendly names lead;
// each row opens the device's full story in a wide drawer.

const leasesPath = "/tmp/dhcp.leases"

// devicesPath is where the roster answers — the sidebar row's destination and
// the home page's doorway, so the three never drift apart.
const devicesPath = "/devices"

// handleDevices renders the roster. It is the shell's own content, so a render
// failure is a real 500 rather than a contained notice; every reading behind it
// degrades on its own (an unreadable source drops its facts, never the page).
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	roster := s.connectedDevices(r.Context(), s.sessionSID(r), clientIP(r))
	configured, limitsOffered, limitsAvailable := s.deviceLimits(r)
	roster = mergeDeviceLimits(roster, configured)

	var body strings.Builder
	lang, t := s.localize(r)
	// The bar belongs to the listing, so the two render together: the cuts it
	// offers are priced from the same roster the rows come from.
	bar := widget.DevicesBar(roster)
	act := widget.DevicesAct(roster, s.EntityListingAct("device"), func(mac string) string {
		if mac == "" {
			return widget.EntityPath("device", "new")
		}
		return widget.EntityPath("device", mac)
	})
	table := widget.DevicesTable(roster, func(d widget.Device) []widget.TableRowAct {
		return s.EntityRowActs("device", d.MAC, d.Name, widget.DeviceActTitles(d))
	})
	page := &widget.Stack{Children: []widget.Widget{bar, table}}
	if !limitsAvailable {
		bar.Tabs = bar.Tabs[:len(bar.Tabs)-1]
		table.Note = "Offline devices stay listed until their lease expires."
		if limitsOffered {
			page.Children = []widget.Widget{bar, &widget.Callout{
				Variant: "warning", Compact: true,
				Body: "Device limits couldn’t be loaded. Some offline devices may be missing. Reload to try again.",
			}, table}
		}
	}
	if err := s.widgets.RenderWithToken(&body, page, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "Devices",
		HeadingAct: s.headingAct(r, act, lang, t),
	}, "wide", nil, template.HTML(body.String()))
}

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

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
	"github.com/we-are-mono/verso/internal/widget"
)

// Connected devices: the roster, from the router's own truth. dnsmasq's
// lease file names who holds an address; the kernel supplies everything
// else, joined by MAC — neighbour entries both families (with real NUD
// confidence), the bridge port the MAC was learned on, and conntrack's
// per-connection byte counters. Friendly names lead; each row opens the
// device's full story in a wide drawer.

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

	// Conntrack totals arrive per address (through the helper — the flow
	// table is root's to read); every address of a device keys back to its
	// MAC, folding both families into one total.
	traffic := map[string]sysstat.DeviceTraffic{}
	if byAddr, err := s.backend.ConnStats(ctx, sid); err != nil {
		log.Printf("verso: devices: conntrack unavailable: %v", err)
	} else {
		traffic = foldTraffic(leases, agg, byAddr)
	}

	var devices []deviceEntry
	for _, l := range leases {
		mac := strings.ToLower(l.mac)
		entries := agg[mac]
		down, up := s.trafHist.Series(deviceAddrs(l, entries))
		if down == nil { // no history yet: a flat baseline the live layer can grow
			down, up = []float64{0, 0}, []float64{0, 0}
		}
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
			Traffic:     traffic[mac],
			Down:        down,
			Up:          up,
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

// foldTraffic groups per-address conntrack totals by device MAC — both
// families of a device summing into one figure.
func foldTraffic(leases []lease, agg map[string][]sysstat.Neighbor, byAddr map[string]sysstat.DeviceTraffic) map[string]sysstat.DeviceTraffic {
	traffic := make(map[string]sysstat.DeviceTraffic)
	for _, l := range leases {
		mac := strings.ToLower(l.mac)
		for _, addr := range deviceAddrs(l, agg[mac]) {
			if at, ok := byAddr[addr]; ok {
				t := traffic[mac]
				t.TxBytes += at.TxBytes
				t.RxBytes += at.RxBytes
				t.Conns += at.Conns
				traffic[mac] = t
			}
		}
	}
	return traffic
}

// deviceAddrs is every address a device answers to: the lease's v4 plus each
// neighbour entry on its MAC.
func deviceAddrs(l lease, entries []sysstat.Neighbor) []string {
	addrs := make([]string, 0, 1+len(entries))
	addrs = append(addrs, l.ip)
	for _, n := range entries {
		if n.Addr != l.ip {
			addrs = append(addrs, n.Addr)
		}
	}
	return addrs
}

// deviceSeries is one device's frame on the overview stream — the same
// series the drawer's panel chart was rendered from (so the live layer
// redraws exactly what the static layer drew), plus its running totals for
// the stat tiles.
type deviceSeries struct {
	Key     string    `json:"key"`
	Down    []float64 `json:"down"`
	Up      []float64 `json:"up"`
	RxBytes int64     `json:"rx"`
	TxBytes int64     `json:"tx"`
	Conns   int       `json:"conns"`
}

// deviceTrafficSeries builds the per-device frames the stream pushes each
// tick, from the tick's own conntrack snapshot. Devices with no observed
// history yet send nothing.
func (s *Server) deviceTrafficSeries(byAddr map[string]sysstat.DeviceTraffic) []deviceSeries {
	raw, err := s.readLeases()
	if err != nil {
		return nil
	}
	leases := parseLeases(raw)
	neigh, _ := s.neighbors()
	agg := aggregateNeighbors(neigh)
	traffic := foldTraffic(leases, agg, byAddr)
	var out []deviceSeries
	for _, l := range leases {
		mac := strings.ToLower(l.mac)
		down, up := s.trafHist.Series(deviceAddrs(l, agg[mac]))
		if down == nil {
			continue
		}
		t := traffic[mac]
		out = append(out, deviceSeries{
			Key: mac, Down: down, Up: up,
			RxBytes: t.RxBytes, TxBytes: t.TxBytes, Conns: t.Conns,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// presenceBadge is the shared presence vocabulary — the roster rows and the
// drawer's address table speak it identically.
func presenceBadge(p presence) *widget.Badge {
	switch p {
	case presenceOnline:
		return &widget.Badge{Variant: "success", Dot: true, Text: "Online"}
	case presenceIdle:
		return &widget.Badge{Variant: "neutral", Dot: true, Text: "Idle"}
	}
	return &widget.Badge{Variant: "neutral", Text: "Offline"}
}

// connectedSection composes the roster as a flat table: one inert row per
// device — its name carrying its zone as an inline chip, its address (mono,
// copyable), a presence pill, and its total download — with the device's full
// story opening from a trailing Details link. The header band names the roster
// and counts what's online.
func connectedSection(devices []deviceEntry) widget.Widget {
	if len(devices) == 0 {
		return nil
	}
	rows := make([]widget.TableRow, 0, len(devices))
	online := 0
	for _, d := range devices {
		if d.Presence == presenceOnline {
			online++
		}
		p := presenceBadge(d.Presence)
		down, unit := splitBytes(d.Traffic.RxBytes)
		rows = append(rows, widget.TableRow{
			ID: d.MAC,
			Cells: []widget.TableCell{
				{Text: d.Name, Chip: d.Zone},
				{Text: d.IP, Copy: true},
				{Text: p.Text, Variant: p.Variant, Dot: p.Dot},
				{Text: down + " " + unit},
			},
			Drawer: &widget.RowDrawer{Title: d.Name, Children: deviceDrawerBody(d)},
		})
	}
	return &widget.Table{
		Style:  "flat",
		Title:  "Connected devices",
		Detail: fmt.Sprintf("%d online", online),
		Columns: []widget.TableColumn{
			{Kind: "name"}, {Kind: "mono"}, {Kind: "pill"}, {Kind: "num"},
		},
		Rows: rows,
	}
}

// deviceDrawerBody is the device's full story — everything the kernel and
// the lease file say, and nothing invented. The order tells it top-down:
// what's flowing right now (the live rates and their minute), the running
// totals, the honest note on what routed traffic can count, then every
// address the device answers to.
func deviceDrawerBody(d deviceEntry) []widget.Widget {
	downTotal, downUnit := splitBytes(d.Traffic.RxBytes)
	upTotal, upUnit := splitBytes(d.Traffic.TxBytes)
	stats := []widget.Widget{
		&widget.Stat{Label: "Downloaded", Value: downTotal, Unit: downUnit, Style: "bare", Name: d.MAC + ":down"},
		&widget.Stat{Label: "Uploaded", Value: upTotal, Unit: upUnit, Style: "bare", Name: d.MAC + ":up"},
		&widget.Stat{Label: "Connections", Value: strconv.Itoa(d.Traffic.Conns), Style: "bare", Name: d.MAC + ":conns"},
	}
	if d.LeaseExpiry > 0 {
		stats = append(stats, &widget.Stat{Label: "Lease renews", Style: "bare",
			Value: strings.TrimPrefix(leaseIn(d.LeaseExpiry, time.Now()), "in ")})
	}

	addrs := make([]widget.Property, 0, 2+len(d.Addrs))
	seen := false
	for _, a := range d.Addrs {
		if a.Addr == d.IP {
			seen = true
		}
		addrs = append(addrs, addrProperty(a.Addr, a.State))
	}
	if !seen { // the lease's v4 even when the kernel holds no entry for it
		addrs = append([]widget.Property{addrProperty(d.IP, 0)}, addrs...)
	}
	addrs = append(addrs, widget.Property{Label: "MAC", Value: d.MAC, Mono: true, Copy: true})
	if d.Port != "" {
		addrs = append(addrs, widget.Property{Label: "Interface", Value: d.Port, Mono: true})
	}

	return []widget.Widget{
		trafficSpark(d),
		&widget.Grid{Columns: len(stats), Children: stats},
		&widget.Callout{Variant: "info",
			Body: "Traffic counts what has crossed the router on connections still open — chatter between devices on your own network never passes through, so it isn't counted."},
		&widget.Properties{Items: addrs, Align: "left"},
	}
}

// trafficSpark draws the device's last minute of throughput — download and
// upload on the fixed-height panel plot, the current rates reading out above
// it — fed by the history the overview stream accumulates and updated live
// by the stream's traffic events. A quiet (or not-yet-observed) minute draws
// in the calm grey.
func trafficSpark(d deviceEntry) widget.Widget {
	idle := true
	for _, v := range append(append([]float64{}, d.Down...), d.Up...) {
		if v > 0.01 {
			idle = false
			break
		}
	}
	return &widget.Chart{
		Size: "panel", Idle: idle, Name: d.MAC,
		Axis: true, Unit: "Mbps", AxisStart: "60s ago", AxisEnd: "now",
		Label: d.Name + " throughput over the last minute",
		Note:  "live · 1s samples",
		Series: []widget.ChartSeries{
			{Label: "Download", Value: rateStr(lastPoint(d.Down)), Values: d.Down, Role: "sky", Fill: true},
			{Label: "Upload", Value: rateStr(lastPoint(d.Up)), Values: d.Up, Role: "violet"},
		},
	}
}

// addrProperty is one address line: its family as the label, the address
// itself with a copy control, and its own presence state in the shared badge
// vocabulary.
func addrProperty(addr string, st sysstat.NeighState) widget.Property {
	label := "IPv4"
	switch {
	case strings.HasPrefix(addr, "fe80"):
		label = "Link-local"
	case strings.Contains(addr, ":"):
		label = "IPv6"
	}
	status := &widget.Badge{Variant: "neutral", Text: "Offline"}
	switch {
	case st.Active():
		status = &widget.Badge{Variant: "success", Dot: true, Text: "Online"}
	case st.Recent():
		status = &widget.Badge{Variant: "neutral", Dot: true, Text: "Idle"}
	}
	return widget.Property{Label: label, Value: addr, Mono: true, Copy: true, Status: status}
}

// splitBytes breaks formatBytes' "103.7 MiB" into the value and its unit,
// the shape a stat tile wants.
func splitBytes(b int64) (value, unit string) {
	parts := strings.SplitN(formatBytes(b), " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// lastPoint is a series' newest value, zero when there is none.
func lastPoint(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	return vals[len(vals)-1]
}

// rateStr renders a current rate in Mbps — always the nearest whole number,
// the readout stays calm.
func rateStr(v float64) string {
	return fmt.Sprintf("%.0f", v)
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

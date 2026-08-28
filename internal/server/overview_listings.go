// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// The overview's Interfaces and DHCP-leases listings, read live from the backend.
// Both reuse the readers the rest of the shell already has — the network topology
// behind the ports panel, and the device roster behind connected devices — so the
// tables can't drift from those views. A source that fails logs and yields no
// rows; the table then renders its header over an empty body, never a 500.

// overviewInterfaces reads the box's ports — the lan bridge members, then the
// uplink — with each one's live carrier, negotiated speed, and RX/TX totals.
func (s *Server) overviewInterfaces(ctx context.Context, sid string) []widget.OverviewInterface {
	cfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: overview: network config unavailable: %v", err)
		return nil
	}
	topo := parseNetworkTopology(cfg)

	rows := make([]widget.OverviewInterface, 0, len(topo.lanPorts)+1)
	add := func(dev string, wan bool) {
		st := s.deviceStats(ctx, sid, dev)
		rows = append(rows, widget.OverviewInterface{
			Port:    dev,
			Up:      st.Carrier,
			Wan:     wan,
			Speed:   linkSpeed(st),
			Traffic: ifaceTraffic(st),
		})
	}
	for _, dev := range topo.lanPorts {
		add(dev, false)
	}
	if topo.wanDevice != "" {
		add(topo.wanDevice, true)
	}
	return rows
}

// ifaceTraffic renders a port's lifetime RX/TX byte counters as "4.1 / 0.9 GB";
// an untouched port (both zero) shows a quiet dash.
func ifaceTraffic(st openwrt.DeviceStats) string {
	if st.RxBytes == 0 && st.TxBytes == 0 {
		return ""
	}
	return gb(st.RxBytes) + " / " + gb(st.TxBytes) + " GB"
}

// overviewLeases reads the DHCP roster: the leaseholders, each with its zone
// chip, MAC, IP, and time left on the lease. A device the kernel knows only from
// the neighbour table (no lease) is not a lease, so it is skipped here.
func (s *Server) overviewLeases(ctx context.Context, sid string) []widget.OverviewLease {
	devs := s.deviceList(ctx, sid)
	now := time.Now().Unix()

	rows := make([]widget.OverviewLease, 0, len(devs))
	for _, d := range devs {
		if d.LeaseExpiry == 0 {
			continue
		}
		name := d.Name
		if name == "" {
			name = "Unknown device"
		}
		rows = append(rows, widget.OverviewLease{
			Name:    name,
			Zone:    d.Zone,
			MAC:     d.MAC,
			IP:      d.IP,
			Expires: leaseRemaining(d.LeaseExpiry, now),
		})
	}
	return rows
}

// leaseRemaining renders time left on a lease as "11h 12m" / "30m"; a lease
// already past its expiry reads "expired", one with no expiry (static) a dash.
func leaseRemaining(expiry, now int64) string {
	if expiry < 0 {
		return ""
	}
	sec := expiry - now
	if sec <= 0 {
		return "expired"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

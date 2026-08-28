// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// The overview's Interfaces and DHCP-leases listings, read live from the backend.
// Both reuse the readers the rest of the shell already has — the network topology
// behind the ports panel, and the device roster behind connected devices — so the
// tables can't drift from those views. A source that fails logs and yields no
// rows; the table then renders its header over an empty body, never a 500.

// overviewPorts reads the box's physical ports — the lan bridge members, then
// the uplink — with each one's live carrier, VLANs, speed, and RX/TX totals.
func (s *Server) overviewPorts(ctx context.Context, sid string) []widget.OverviewPort {
	cfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: overview: network config unavailable: %v", err)
		return nil
	}
	topo := parseNetworkTopology(cfg)
	vlansByPort := portVLANs(parseBridgeVLANs(cfg))

	rows := make([]widget.OverviewPort, 0, len(topo.lanPorts)+1)
	add := func(dev string, wan bool) {
		st := s.deviceStats(ctx, sid, dev)
		rows = append(rows, widget.OverviewPort{
			Port:    dev,
			Up:      st.Carrier,
			Wan:     wan,
			VLANs:   vlansByPort[dev],
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

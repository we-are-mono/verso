// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// The gateway card: the physical rear panel on the overview. The port list
// comes from what the network config declares — the lan bridge's members,
// then the wan interface's device — and each connector is lit from live
// device state (carrier, negotiated speed). One builder feeds the page render
// and the overview stream's `ports` events, so the panel and the pushed
// updates cannot drift.

// portActivityBytes is the per-sample traffic floor for the amber LED: below
// it, background murmur (ARP, discovery) reads as idle.
const portActivityBytes = 512

// portList assembles the panel: lan members as "Network N" in stable order,
// the uplink as "Internet" last. The second return carries each port's
// current rx+tx byte total, the raw material for activity marking
// (markPortActivity) between two of these. Empty when the network config is
// unreadable — the page then simply carries no panel.
func (s *Server) portList(ctx context.Context, sid string) ([]widget.PortItem, map[string]int64) {
	cfg, err := s.backend.UCIConfig(ctx, sid, "network")
	if err != nil {
		log.Printf("verso: ports: network config unavailable: %v", err)
		return nil, nil
	}
	topo := parseNetworkTopology(cfg)

	items := make([]widget.PortItem, 0, len(topo.lanPorts)+1)
	counters := make(map[string]int64, len(topo.lanPorts)+1)
	for i, dev := range topo.lanPorts {
		st := s.deviceStats(ctx, sid, dev)
		counters[dev] = st.RxBytes + st.TxBytes
		items = append(items, widget.PortItem{
			Kind: "rj45", Label: fmt.Sprintf("Network %d", i+1), Role: "lan",
			Linked: st.Carrier, Speed: linkSpeed(st), Iface: dev, Addr: topo.lanAddr,
		})
	}
	if topo.wanDevice != "" {
		ws, err := s.backend.WANStatus(ctx, sid)
		if err != nil {
			log.Printf("verso: ports: wan status unavailable: %v", err)
		}
		st := s.deviceStats(ctx, sid, topo.wanDevice)
		counters[topo.wanDevice] = st.RxBytes + st.TxBytes
		items = append(items, widget.PortItem{
			Kind: "rj45", Label: "Internet", Role: "wan",
			Linked: st.Carrier, Speed: linkSpeed(st), Iface: topo.wanDevice,
			Addr: ws.Addr, Note: protoNote(topo.wanProto),
		})
	}
	return items, counters
}

// markPortActivity lights the amber LED on ports whose byte total moved past
// the floor since the previous counters. A nil prev (the first sample) marks
// nothing — activity is a delta, and one sample has none.
func markPortActivity(items []widget.PortItem, counters, prev map[string]int64) {
	if prev == nil {
		return
	}
	for i := range items {
		delta := counters[items[i].Iface] - prev[items[i].Iface]
		items[i].Active = delta >= portActivityBytes
	}
}

// deviceStats reads one device's live state, degrading to zero state (unlit,
// no speed) rather than dropping the connector — the port exists even when
// its stats cannot be read.
func (s *Server) deviceStats(ctx context.Context, sid, dev string) openwrt.DeviceStats {
	st, err := s.backend.DeviceStats(ctx, sid, dev)
	if err != nil {
		log.Printf("verso: ports: device %s stats unavailable: %v", dev, err)
		return openwrt.DeviceStats{}
	}
	return st
}

// networkTopology is the panel-relevant slice of the uci network config.
type networkTopology struct {
	wanDevice string   // the uplink's configured device
	wanProto  string   // its protocol ("dhcp", "pppoe", "static")
	lanAddr   string   // the lan interface's address (every lan port shares it)
	lanPorts  []string // the lan bridge's member devices, sorted
}

// parseNetworkTopology reads the wan and lan interface sections and resolves
// the lan device: a bridge contributes its member ports, a plain device
// contributes itself.
func parseNetworkTopology(cfg map[string]any) networkTopology {
	var topo networkTopology
	lanDevice := ""
	for _, v := range cfg {
		section, ok := v.(map[string]any)
		if !ok || section[".type"] != "interface" {
			continue
		}
		device, _ := section["device"].(string)
		switch section[".name"] {
		case "wan":
			topo.wanDevice = device
			topo.wanProto, _ = section["proto"].(string)
		case "lan":
			lanDevice = device
			topo.lanAddr, _ = section["ipaddr"].(string)
		}
	}
	if lanDevice == "" {
		return topo
	}
	for _, v := range cfg {
		section, ok := v.(map[string]any)
		if !ok || section[".type"] != "device" {
			continue
		}
		if name, _ := section["name"].(string); name != lanDevice {
			continue
		}
		if members, ok := section["ports"].([]any); ok {
			for _, m := range members {
				if port, ok := m.(string); ok {
					topo.lanPorts = append(topo.lanPorts, port)
				}
			}
			sort.Strings(topo.lanPorts)
			return topo
		}
	}
	// No bridge section: the lan interface sits on the device directly.
	topo.lanPorts = []string{lanDevice}
	return topo
}

// gatewayPanel is the rear panel as the overview carries it: bare on the
// canvas (no card frame — the chassis is its own object), a hairline rule
// below setting it apart from what follows.
func gatewayPanel(items []widget.PortItem) *widget.Ports {
	return &widget.Ports{Accent: "sky", Legend: true, Items: items}
}

// linkSpeed renders a negotiated link the way a person says it; an unlinked
// or speedless port shows a quiet dash.
func linkSpeed(st openwrt.DeviceStats) string {
	if !st.Carrier || st.SpeedMbps <= 0 {
		return "—"
	}
	if st.SpeedMbps%1000 == 0 {
		return fmt.Sprintf("%d Gbps", st.SpeedMbps/1000)
	}
	return fmt.Sprintf("%d Mbps", st.SpeedMbps)
}

// protoNote names the uplink's protocol for the hover detail.
func protoNote(proto string) string {
	switch proto {
	case "dhcp":
		return "DHCP"
	case "pppoe":
		return "PPPoE"
	case "static":
		return "Static"
	}
	return ""
}

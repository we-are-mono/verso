// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"log"
	"net/http"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/units"
	"github.com/we-are-mono/verso/internal/widget"
)

// The overview stream: one long-lived GET (Server-Sent Events) the browser's
// EventSource holds open, into which the shell pushes fresh truth — the
// server owns the sampling clock, the client just renders what arrives. System
// meters, interfaces, WAN traffic, sensors, clock and the overview verdict share
// this connection without creating a sampler per
// browser tab.

// handleOverviewEvents serves the stream on the sampling clock (serveSSE): a
// stream must not outlive its session the way a one-shot poll could not have.
//
// It follows the browser's card layout (ADR-022), read once per connection: a
// frame only hidden cards would read is not sent, and a reading only such a
// frame needs is not taken. What the verdict counts is read whatever is
// hidden, so hiding a card never makes the page read "online". The page
// reconnects when a card is hidden or comes back.
func (s *Server) handleOverviewEvents(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	_, catalog := s.localize(r)
	tr := translatorOrIdentity(catalog)
	layout := s.layouts.Layout(r)
	liveOverview := &widget.Overview{}

	// What a connection reads once: the board, and the interfaces' topology,
	// which changes only with an apply, after which the page is read anew.
	boardName := ""
	if b, err := s.backend.Board(r.Context(), sid); err == nil {
		boardName = b.BoardName
		liveOverview.Firmware, liveOverview.Kernel, liveOverview.Model = b.Firmware, b.Kernel, b.Model
	}
	var topology []widget.OverviewInterface
	if snapshot, ok := s.telemetrySnapshot(r.Context()); ok {
		wan, _ := s.backend.WANStatus(r.Context(), sid)
		topology = s.interfaceList(r.Context(), sid, snapshot, wan)
	}

	sendMeters := func() bool {
		now := clock.Now()
		liveOverview.Uptime = loginDuration(tr, loginUptime())
		if writeEvent(w, "", "clock", map[string]string{"clock": now.Format("15:04:05"), "uptime": liveOverview.Uptime}) != nil {
			return false
		}
		// The verdict counts a resource out of band, so the readings are taken
		// even while Resources is hidden; only their frame is not sent.
		readings := s.systemMeters(r.Context(), sid, tr)
		liveOverview.SysMetrics = sysMetricsToWidget(readings)
		if !layout.Shows("resources") {
			return true
		}
		return writeEvent(w, "", "meters", map[string]any{"meters": readings}) == nil
	}
	sendInterfaces := func() bool {
		snapshot, ok := s.telemetrySnapshot(r.Context())
		liveOverview.InterfacesKnown = ok
		liveOverview.Interfaces = nil
		if !ok {
			return true
		}
		liveOverview.Interfaces = liveInterfaces(topology, snapshot)
		if !layout.Shows("ports") && !layout.Shows("tunnels") {
			return true
		}
		liveOverview.DevicesOnline, liveOverview.DevicesKnown = s.onlineDevices()
		readings := telemetryInterfaceReadings(snapshot, units.For(catalog))
		for i := range readings {
			readings[i].State = tr(readings[i].State)
		}
		return writeEvent(w, "", "interfaces", map[string]any{"interfaces": readings}) == nil
	}
	// The WAN throughput frame: sample the uplink device's counters once a
	// second, fold them into the minute-long history the overview graph draws,
	// and stream the newest down/up so the live graph scrolls in real values.
	// The uplink's state is the verdict's, so it is read whatever is hidden.
	// Its address, the Internet card's line, changes only when the uplink
	// connects anew, so it is read then: on the first tick, and whenever the
	// device changes or its uptime starts over.
	var addressDevice string
	var addressUptime int64
	sendWan := func() bool {
		ws, err := s.backend.WANStatus(r.Context(), sid)
		liveOverview.WANKnown, liveOverview.WANUp = err == nil, err == nil && ws.Up()
		liveOverview.WANDevice, liveOverview.WANUptime = "", ""
		if err != nil {
			return true // no uplink is a page-render concern, not a stream killer
		}
		primary, found := ws.Primary()
		if found {
			liveOverview.WANDevice, liveOverview.WANUptime = primary.Device, loginDuration(tr, primary.Uptime)
			if addressDevice == "" || primary.Device != addressDevice || primary.Uptime < addressUptime {
				liveOverview.WANAddress = ""
				if conn, err := s.backend.WANConn(r.Context(), sid); err == nil {
					liveOverview.WANAddress = conn.V4Addr
				}
				addressDevice = primary.Device
			}
			addressUptime = primary.Uptime
		}
		if !layout.Shows("traffic") && !layout.Shows("internet") {
			return true
		}
		write := func(down, up float64) bool {
			liveOverview.DownNow, liveOverview.UpNow = down, up
			// connected is how long the uplink has held, written for the
			// Internet card's row.
			return writeEvent(w, "", "wan", map[string]any{"down": down, "up": up, "uptime": primary.Uptime, "connected": liveOverview.WANUptime}) == nil
		}
		if !found || primary.Device == "" {
			return write(0, 0)
		}
		if snapshot, ok := s.telemetrySnapshot(r.Context()); ok {
			if device, found := snapshot.Interface(primary.Device); found && len(device.History) != 0 {
				point := device.History[len(device.History)-1]
				return write(rateMbps(point.RxBPS), rateMbps(point.TxBPS))
			}
		}
		// Without a telemetry sample, read the uplink's byte counters directly;
		// an unreadable device counts as zero rather than failing the frame.
		st, err := s.backend.DeviceStats(r.Context(), sid, primary.Device)
		if err != nil {
			log.Printf("verso: device %s stats unavailable: %v", primary.Device, err)
			st = openwrt.DeviceStats{}
		}
		s.wanHist.Observe(st.RxBytes, st.TxBytes)
		down, up := s.wanHist.Latest()
		return write(down, up)
	}
	// The hardware-sensor frame: CPU temperature, fan speed, power draw, resolved
	// through the board profile and pushed each readings tick so the Hardware
	// card's rows keep current the way the gauges do. Nothing reads them while
	// Hardware is hidden, so then the sensors are left alone.
	sendSensors := func() bool {
		if !layout.Shows("hardware") {
			return true
		}
		v := resolveSensors(tr, boardName)
		liveOverview.Temperature, liveOverview.TempDot, liveOverview.Fan, liveOverview.Power = v.Temperature, v.TempLevel, v.Fan, v.Power
		return writeEvent(w, "", "sensors", v) == nil
	}
	sendOverview := func() bool {
		s.applyFirewallStatus(r.Context(), sid, liveOverview)
		return writeEvent(w, "", "overview", liveOverview.LiveStatus(tr)) == nil
	}
	s.serveSSE(w, r, func() bool {
		return sendMeters() && sendInterfaces() && sendWan() && sendSensors() && sendOverview()
	})
}

// liveInterfaces is this second's interfaces in the topology the connection
// read: each keeps its networks, relations and kind, and takes its state and
// rates from the snapshot. An interface that appeared since stands bare.
func liveInterfaces(topology []widget.OverviewInterface, snapshot telemetry.Snapshot) []widget.OverviewInterface {
	known := make(map[string]widget.OverviewInterface, len(topology))
	for _, n := range topology {
		known[n.Name] = n
	}
	out := make([]widget.OverviewInterface, 0, len(snapshot.Interfaces))
	for _, iface := range snapshot.Interfaces {
		n, ok := known[iface.Name]
		if !ok {
			n = widget.OverviewInterface{Name: iface.Name, Kind: iface.Kind, Physical: iface.Physical, Conduit: iface.Conduit}
		}
		n.State = normalOperstate(iface.Operstate)
		if len(iface.History) != 0 {
			point := iface.History[len(iface.History)-1]
			n.RxBPS, n.TxBPS = point.RxBPS, point.TxBPS
		}
		out = append(out, n)
	}
	return out
}

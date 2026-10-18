// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/machine"
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
	if reads, ok := r.URL.Query()["reads"]; ok {
		s.servePageFrames(w, r, pageFramesNamed(reads))
		return
	}
	sid := s.sessionSID(r)
	_, catalog := s.localize(r)
	tr := translatorOrIdentity(catalog)
	layout := s.layouts.Layout(r)
	liveOverview := &widget.Overview{Layout: layout}

	// What a connection reads once: the board, and the interfaces' topology,
	// which changes only with an apply, after which the page is read anew.
	if b, err := s.backend.Board(r.Context(), sid); err == nil {
		liveOverview.Firmware, liveOverview.Kernel, liveOverview.Model = b.Firmware, b.Kernel, b.Model
	}
	var topology []widget.OverviewInterface
	if snapshot, ok := s.telemetrySnapshot(r.Context()); ok {
		wan, _ := s.backend.WANStatus(r.Context(), sid)
		topology = s.interfaceList(r.Context(), sid, snapshot, wan)
	}

	sendMeters := func() bool {
		now := clock.Now()
		uptime := loginUptime()
		liveOverview.Uptime = &uptime
		if writeEvent(w, "", "clock", map[string]any{"time": widget.LocalTime(now), "uptime": units.Duration(uptime)}) != nil {
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
		// Each frame reads the table afresh, so it asks the grant afresh.
		liveOverview.DevicesOnline, liveOverview.DevicesKnown = s.onlineDevices(r.Context(), s.grantsFor(sid))
		return writeEvent(w, "", "interfaces", interfacesFrame(snapshot, catalog)) == nil
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
		liveOverview.WANDevice, liveOverview.WANUptime = "", nil
		if err != nil {
			return true // no uplink is a page-render concern, not a stream killer
		}
		primary, found := ws.Primary()
		if found {
			connected := primary.Uptime
			liveOverview.WANDevice, liveOverview.WANUptime = primary.Device, &connected
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
			return writeEvent(w, "", "wan", map[string]any{"down": down, "up": up, "uptime": primary.Uptime, "connected": units.Duration(primary.Uptime)}) == nil
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
	// The board's sensors, for the Hardware card's rows and header and for the
	// heading: the sampler reads them once per five seconds for every
	// connection, the card hidden or not, because the heading counts a board
	// past its limits. Only the card's rows wait on it being shown.
	// What needs attention beyond the readings changes over weeks, not
	// seconds, so it is read once for the connection.
	hwAlerts := hardwareAlerts((&machine.Reader{}).Read(), s.hardwareState(r.Context(), sid))
	sendSensors := func() bool {
		if s.boardSensors == nil {
			return true
		}
		liveOverview.Hardware = hardwareView(s.boardSensors.Read(), hwAlerts)
		if !layout.Shows("hardware") {
			return true
		}
		return writeEvent(w, "", "sensors", map[string]any{"rows": hardwareRows(liveOverview.Hardware)}) == nil
	}
	frames := 0
	sendOverview := func() bool {
		s.applyFirewallStatus(r.Context(), sid, liveOverview)
		// Neither changes but on an apply, so they are read at the slower
		// pace, never every frame.
		if frames%serviceStatusEvery == 0 {
			s.applyFailedServices(r.Context(), sid, liveOverview)
			s.applyExposure(r, liveOverview)
		}
		frames++
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

// interfacesFrame is the interface stream's frame: each interface's state,
// rates, link, activity and speed, its state in the reader's language.
func interfacesFrame(snapshot telemetry.Snapshot, catalog func(string) string) map[string]any {
	tr := translatorOrIdentity(catalog)
	readings := telemetryInterfaceReadings(snapshot, units.For(catalog))
	for i := range readings {
		readings[i].State = tr(readings[i].State)
	}
	return map[string]any{"interfaces": readings}
}

// pageFrames are the frames a page other than the Overview may read on their
// own, by naming them in ?reads=: System › Hardware draws the router's panel
// and reads only the interfaces.
var pageFrames = map[string]bool{"interfaces": true}

// pageFramesNamed is the allowed frames among those a page named, in one or
// more comma-separated reads values; any other name is ignored.
func pageFramesNamed(reads []string) map[string]bool {
	named := map[string]bool{}
	for _, value := range reads {
		for _, name := range strings.Split(value, ",") {
			if pageFrames[name] {
				named[name] = true
			}
		}
	}
	return named
}

// servePageFrames streams only the frames a page named, on the same clock as
// the Overview's stream. It reads nothing else, and no layout: the page is not
// the Overview.
func (s *Server) servePageFrames(w http.ResponseWriter, r *http.Request, frames map[string]bool) {
	_, catalog := s.localize(r)
	s.serveSSE(w, r, func() bool {
		if frames["interfaces"] {
			if snapshot, ok := s.telemetrySnapshot(r.Context()); ok {
				return writeEvent(w, "", "interfaces", interfacesFrame(snapshot, catalog)) == nil
			}
		}
		return true
	})
}

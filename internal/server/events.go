// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
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
func (s *Server) handleOverviewEvents(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	_, catalog := s.localize(r)
	tr := translatorOrIdentity(catalog)
	liveOverview := &widget.Overview{SecurityHref: s.navLabelHref("Firewall")}

	sendMeters := func() bool {
		now := time.Now()
		if writeEvent(w, "", "clock", map[string]string{"clock": now.Format("15:04:05"), "uptime": loginDuration(tr, loginUptime())}) != nil {
			return false
		}
		readings := s.systemMeters(r.Context(), sid, tr)
		liveOverview.SysMetrics = sysMetricsToWidget(readings)
		return writeEvent(w, "", "meters", map[string]any{"meters": readings}) == nil
	}
	sendInterfaces := func() bool {
		snapshot, ok := s.telemetrySnapshot(r.Context())
		liveOverview.InterfacesKnown = ok
		liveOverview.Interfaces = nil
		if !ok {
			return true
		}
		for _, iface := range snapshot.Interfaces {
			liveOverview.Interfaces = append(liveOverview.Interfaces, widget.OverviewInterface{Name: iface.Name, Kind: iface.Kind, State: normalOperstate(iface.Operstate), Physical: iface.Physical})
		}
		liveOverview.DevicesOnline, liveOverview.DevicesKnown = s.onlineDevices()
		readings := telemetryInterfaceReadings(snapshot)
		for i := range readings {
			readings[i].State = tr(readings[i].State)
		}
		return writeEvent(w, "", "interfaces", map[string]any{"interfaces": readings}) == nil
	}
	// The WAN throughput frame: sample the uplink device's counters once a
	// second, fold them into the minute-long history the overview graph draws,
	// and stream the newest down/up so the live graph scrolls in real values.
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
		}
		write := func(down, up float64) bool {
			caption := ""
			if found {
				caption = fmt.Sprintf(tr("for %s"), loginDuration(tr, primary.Uptime))
			}
			return writeEvent(w, "", "wan", map[string]any{"down": down, "up": up, "uptime": primary.Uptime, "uptime_label": caption}) == nil
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
	// through the board profile and pushed each readings tick so the System panel's
	// rows keep current the way the gauges above them do. The board name selects
	// the profile and is fixed for the box, so read it once per connection.
	boardName := ""
	if b, err := s.backend.Board(r.Context(), sid); err == nil {
		boardName = b.BoardName
	}
	sendSensors := func() bool {
		return writeEvent(w, "", "sensors", resolveSensors(tr, boardName)) == nil
	}
	sendOverview := func() bool {
		s.applyFirewallStatus(r.Context(), sid, liveOverview)
		return writeEvent(w, "", "overview", liveOverview.LiveStatus(tr)) == nil
	}
	s.serveSSE(w, r, func() bool {
		return sendMeters() && sendInterfaces() && sendWan() && sendSensors() && sendOverview()
	})
}

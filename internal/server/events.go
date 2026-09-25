// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/we-are-mono/verso/internal/widget"
)

// The overview stream: one long-lived GET (Server-Sent Events) the browser's
// EventSource holds open, into which the shell pushes fresh truth — the
// server owns the sampling clock, the client just renders what arrives. System
// meters, interfaces, WAN traffic, sensors, clock and the overview verdict share
// this connection without creating a sampler per
// browser tab.

// handleOverviewEvents serves the stream. The session is re-checked every
// tick: a stream must not outlive its session the way a one-shot poll could
// not have. The check leaves the idle clock where it is — a page holding this
// connection open is the browser working, not a person, and it must not keep
// the router signed in past the expiry the page itself states. When the session
// ends, EventSource's reconnect lands on the login redirect — not an event
// stream — which closes the client for good.
func (s *Server) handleOverviewEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	sid := s.sessionSID(r)
	_, catalog := s.localize(r)
	tr := translatorOrIdentity(catalog)
	liveOverview := &widget.Overview{SecurityHref: s.navLabelHref("Firewall")}
	meters := time.NewTicker(s.eventInterval)
	defer meters.Stop()

	sendMeters := func() bool {
		now := time.Now()
		payload, err := json.Marshal(map[string]string{"clock": now.Format("15:04:05"), "uptime": loginDuration(tr, loginUptime())})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: clock\ndata: %s\n\n", payload); err != nil {
			return false
		}
		readings := s.systemMeters(r.Context(), sid, tr)
		liveOverview.SysMetrics = sysMetricsToWidget(readings)
		return writeMetersEvent(w, readings) == nil
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
		payload, err := json.Marshal(map[string]any{"interfaces": readings})
		if err != nil {
			return true
		}
		_, err = fmt.Fprintf(w, "event: interfaces\ndata: %s\n\n", payload)
		return err == nil
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
			payload, marshalErr := json.Marshal(map[string]any{"down": down, "up": up, "uptime": primary.Uptime, "uptime_label": caption})
			if marshalErr != nil {
				return false
			}
			_, err = fmt.Fprintf(w, "event: wan\ndata: %s\n\n", payload)
			return err == nil
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
		st := s.deviceStats(r.Context(), sid, primary.Device)
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
		payload, err := json.Marshal(resolveSensors(tr, boardName))
		if err != nil {
			return true // a formatting slip is not a stream killer
		}
		_, err = fmt.Fprintf(w, "event: sensors\ndata: %s\n\n", payload)
		return err == nil
	}
	sendOverview := func() bool {
		s.applyFirewallStatus(r.Context(), sid, liveOverview)
		payload, err := json.Marshal(liveOverview.LiveStatus(tr))
		if err != nil {
			return false
		}
		_, err = fmt.Fprintf(w, "event: overview\ndata: %s\n\n", payload)
		return err == nil
	}
	if !sendMeters() || !sendInterfaces() || !sendWan() || !sendSensors() || !sendOverview() {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done(): // the browser went away
			return
		case <-meters.C:
			if !s.sessionAlive(r) || !sendMeters() || !sendInterfaces() || !sendWan() || !sendSensors() || !sendOverview() {
				return
			}
			flusher.Flush()
		}
	}
}

// writeMetersEvent frames one readings snapshot as an SSE `meters` event:
// `event:` names the type, `data:` carries the JSON, the blank line ends it.
func writeMetersEvent(w io.Writer, readings []meterReading) error {
	payload, err := json.Marshal(map[string]any{"meters": readings})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: meters\ndata: %s\n\n", payload)
	return err
}

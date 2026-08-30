// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The overview stream: one long-lived GET (Server-Sent Events) the browser's
// EventSource holds open, into which the shell pushes fresh truth — the
// server owns the sampling clock, the client just renders what arrives. Four
// event types ride it: system meters, kernel interfaces, WAN traffic, and
// sensors. Further types join the same stream without creating a sampler per
// browser tab.

// handleOverviewEvents serves the stream. The session is re-checked every
// tick: a stream must not outlive its session the way a one-shot poll could
// not have. When it ends, EventSource's reconnect lands on the login
// redirect — not an event stream — which closes the client for good.
func (s *Server) handleOverviewEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	sid := s.sessionSID(r)
	meters := time.NewTicker(s.eventInterval)
	defer meters.Stop()

	sendMeters := func() bool {
		return writeMetersEvent(w, s.systemMeters(r.Context(), sid)) == nil
	}
	sendInterfaces := func() bool {
		snapshot, ok := s.telemetrySnapshot(r.Context())
		if !ok {
			return true
		}
		payload, err := json.Marshal(map[string]any{"interfaces": telemetryInterfaceReadings(snapshot)})
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
		if err != nil {
			return true // no uplink is a page-render concern, not a stream killer
		}
		write := func(down, up float64) bool {
			_, err = fmt.Fprintf(w, "event: wan\ndata: {\"down\":%.2f,\"up\":%.2f,\"uptime\":%d}\n\n", down, up, ws.Uptime)
			return err == nil
		}
		if ws.Device == "" {
			return write(0, 0)
		}
		if snapshot, ok := s.telemetrySnapshot(r.Context()); ok {
			if device, found := snapshot.Interface(ws.Device); found && len(device.History) != 0 {
				point := device.History[len(device.History)-1]
				return write(rateMbps(point.RxBPS), rateMbps(point.TxBPS))
			}
		}
		st := s.deviceStats(r.Context(), sid, ws.Device)
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
		payload, err := json.Marshal(resolveSensors(boardName))
		if err != nil {
			return true // a formatting slip is not a stream killer
		}
		_, err = fmt.Fprintf(w, "event: sensors\ndata: %s\n\n", payload)
		return err == nil
	}
	if !sendMeters() || !sendInterfaces() || !sendWan() || !sendSensors() {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done(): // the browser went away
			return
		case <-meters.C:
			if s.sessionUser(r) == "" || !sendMeters() || !sendInterfaces() || !sendWan() || !sendSensors() {
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

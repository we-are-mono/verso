// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The overview stream: one long-lived GET (Server-Sent Events) the browser's
// EventSource holds open, into which the shell pushes fresh truth — the
// server owns the sampling clock, the client just renders what arrives. Two
// event types ride it: `meters` every second; `ports` whenever the panel's
// truth moves (a cable, a renegotiation, traffic starting or stopping) —
// sampled on its own faster clock so the activity LEDs feel live; and
// `traffic`, the per-device rate history behind the drawer charts, sent on
// the readings tick while it changes. Further types (an uplink going down, a
// lease appearing) join the same stream.

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
	// Ports run their own, faster clock: the amber activity LED wants a
	// livelier signal than the once-a-second readings, and a port sample is
	// one cheap device read.
	ports := time.NewTicker(s.portsInterval)
	defer ports.Stop()

	var lastPorts, lastTraffic []byte
	var prevCounters map[string]int64
	sendMeters := func() bool {
		return writeMetersEvent(w, s.meterReadings(r.Context(), sid)) == nil
	}
	// Per-device frames follow the same change-driven shape as ports: a fully
	// quiet minute stops them until traffic moves again. The tick's conntrack
	// snapshot serves twice — the rate history behind the charts, and the
	// running totals on the tiles. The stream is the traffic historian: it
	// holds a session and a steady clock.
	sendTraffic := func() bool {
		byAddr, err := s.backend.ConnStats(r.Context(), sid)
		if err != nil {
			return true // conntrack down is a logged page-render concern, not a stream killer
		}
		s.trafHist.Observe(byAddr)
		payload, changed := marshalIfChanged(s.deviceTrafficSeries(byAddr), lastTraffic)
		if !changed {
			return true
		}
		lastTraffic = payload
		_, err = fmt.Fprintf(w, "event: traffic\ndata: {\"devices\":%s}\n\n", payload)
		return err == nil
	}
	// The panel frame goes out only when the truth moved (a cable, a
	// renegotiation, the amber LED flipping) — the change-driven shape every
	// event type after meters follows.
	sendPorts := func() bool {
		items, counters := s.portList(r.Context(), sid)
		markPortActivity(items, counters, prevCounters)
		prevCounters = counters
		payload, changed := marshalIfChanged(items, lastPorts)
		if !changed {
			return true
		}
		lastPorts = payload
		_, err := fmt.Fprintf(w, "event: ports\ndata: {\"ports\":%s}\n\n", payload)
		return err == nil
	}

	if !sendMeters() || !sendPorts() || !sendTraffic() {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done(): // the browser went away
			return
		case <-meters.C:
			if s.sessionUser(r) == "" || !sendMeters() || !sendTraffic() {
				return
			}
			flusher.Flush()
		case <-ports.C:
			if !sendPorts() {
				return
			}
			flusher.Flush()
		}
	}
}

// marshalIfChanged encodes a non-empty value and reports whether it differs
// from the previous encoding — the skip-unchanged-frames helper.
func marshalIfChanged[T any](items []T, last []byte) ([]byte, bool) {
	if len(items) == 0 {
		return last, false
	}
	payload, err := json.Marshal(items)
	if err != nil || bytes.Equal(payload, last) {
		return last, false
	}
	return payload, true
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

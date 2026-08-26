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
// server owns the sampling clock, the client just renders what arrives.
// Today it carries one event type, `meters`, once a second; event-shaped
// facts (an uplink going down, a lease appearing) join as their own types
// and arrive the moment the source reports them, not on the next tick.

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
	ticker := time.NewTicker(s.eventInterval)
	defer ticker.Stop()
	for {
		if s.sessionUser(r) == "" {
			return
		}
		if err := writeMetersEvent(w, s.meterReadings(r.Context(), sid)); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done(): // the browser went away
			return
		case <-ticker.C:
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

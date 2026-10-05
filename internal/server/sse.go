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

// writeEvent frames v as one Server-Sent Event: an `id:` line when id is set
// (what the browser hands back as Last-Event-ID on reconnect), `event:` naming
// the type, `data:` carrying the JSON, and the blank line that ends it.
func writeEvent(w io.Writer, id, name string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if id != "" {
		_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, name, payload)
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, payload)
	return err
}

// serveSSE holds one event stream open: send writes a frame now and on every
// eventInterval tick after, each flushed to the browser, until the browser goes
// away, send reports a failed write, or the session ends. The session is
// re-checked every tick without touching its idle clock: a connection the
// browser holds open must not outlive the sign-in, and holding it open is the
// page working, not a person, so it must not extend the sign-in either. When the
// session ends, EventSource's reconnect lands on the login redirect instead of a
// stream, which closes the client for good.
func (s *Server) serveSSE(w http.ResponseWriter, r *http.Request, send func() bool) {
	s.serveSSEEvery(w, r, s.eventInterval, send)
}

// serveSSEEvery is serveSSE at a pace of the caller's: a stream someone is
// watching line by line, as a run's output is, ticks faster than a reading
// that only moves on.
func (s *Server) serveSSEEvery(w http.ResponseWriter, r *http.Request, every time.Duration, send func() bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	if !send() {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done(): // the browser went away
			return
		case <-ticker.C:
			if !s.sessionAlive(r) || !send() {
				return
			}
			flusher.Flush()
		}
	}
}

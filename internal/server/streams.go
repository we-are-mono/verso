// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// Live listings (ADR-005 §7): a table declares that its rows arrive over time
// and names the source; the shell owns everything else. One long-lived GET per
// open listing, Server-Sent Events on it, and structured rows on the wire —
// never markup. The browser builds the rows itself, which is what keeps a log
// line's contents (a rule name, a hostname, an address a stranger chose) text.
//
// Widget sources are declared with the widget contract. The core Logs page
// uses a separate payload and renderer, registered here by the shell.

const (
	// fwLogBacklog is how many past verdicts a freshly opened listing is given.
	// A live log that opens blank makes a person wait to learn anything; one
	// that dumps the whole ring buries the present under the past.
	fwLogBacklog = 50
	// fwLogBurst bounds one frame. A scanner can produce verdicts faster than a
	// browser can lay out rows; the ring drops the excess either way, so the
	// frame does it first.
	fwLogBurst = 100
	// fwLogMapTTL is how long the zone and rule mappings are trusted. They come
	// from configuration and netifd, which change on a person's clock, not on
	// the packets'.
	fwLogMapTTL = 30 * time.Second
)

// handleStream serves one live listing's source. Unknown sources are not
// served: the set is closed, and a name outside it is a page asking for
// something that does not exist.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	if source != systemLogSource && !widget.StreamSourceKnown(source) {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	stream := s.streamHandler(source)
	if stream == nil {
		// The widget's set said this name may be asked for and nothing here
		// answers it: a source added on one side only. Refusing states that,
		// where an open stream that never speaks would be indistinguishable from
		// a network with nothing to say.
		log.Printf("verso: live listing %q is a declared source with no handler; refusing rather than serving silence", source)
		http.Error(w, "stream unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	stream(w, r, flusher)
}

// streamHandler is the other half of the closed source set: StreamSourceKnown
// says which names may be asked for, this says what answers each one. A name
// with no answer returns nil rather than falling through, so the two halves
// cannot drift into a stream that serves nothing.
func (s *Server) streamHandler(source string) func(http.ResponseWriter, *http.Request, http.Flusher) {
	switch source {
	case systemLogSource:
		return s.streamSystemLog
	case widget.StreamSourceFirewallLog:
		return s.streamFirewallLog
	}
	return nil
}

// streamFirewallLog reads the dedicated NFLOG buffer on the sampling clock,
// resolves packet metadata into rows, and reports collector health separately.
//
// The session is re-checked every tick, exactly as the overview stream does: a
// connection the browser holds open must not outlive the sign-in, and holding
// it open must not extend the sign-in either — this is the page working, not a
// person. When the session ends, EventSource's reconnect lands on the login
// redirect instead of a stream, which closes the client for good.
func (s *Server) streamFirewallLog(w http.ResponseWriter, r *http.Request, flusher http.Flusher) {
	sid := s.sessionSID(r)
	ticker := time.NewTicker(s.eventInterval)
	defer ticker.Stop()

	reader := &fwLogReader{backend: s.backend, sid: sid, limit: fwLogBacklog}
	// A dropped connection is reconnected by the browser on its own, and it
	// hands back the id of the last event it saw. Resuming there is what keeps
	// a reconnect from replaying the backlog into a page that already has it.

	if epoch, cursor, ok := parseFirewallCursor(r.Header.Get("Last-Event-ID")); ok {
		reader.generation, reader.cursor, reader.started = epoch, cursor, true
	}
	send := func() bool {
		rows := reader.poll(r.Context())
		payload, err := json.Marshal(map[string]any{"rows": rows, "reset": reader.reset, "available": reader.available, "lost": reader.lost})
		if err != nil {
			return false
		}
		// Epoch plus ID prevents a restarted collector from replaying old row IDs.
		if reader.generation != "" {
			if _, err = fmt.Fprintf(w, "id: %s:%d\n", reader.generation, reader.cursor); err != nil {
				return false
			}
		}
		_, err = fmt.Fprintf(w, "event: stream\ndata: %s\n\n", payload)
		return err == nil
	}
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

// fwLogReader is one connection's view of the log ring: how far it has read,
// and the mappings it resolves lines through. It is per-connection on purpose —
// the cursor belongs to a reader, not to the device.
type fwLogReader struct {
	backend  openwrt.Backend
	sid      string
	cursor   int64 // the highest log id already sent; only newer records are read
	started  bool  // the first poll is the backlog, every later one is the present
	limit    int   // how many rows the first poll may hand over
	resolver *fwResolver
	mapped   time.Time
	// Report the first read failure to diagnostics, without repeating it every
	// polling tick. The browser independently receives collector availability.
	reported   bool
	generation string
	available  bool
	reset      bool
	lost       bool
	overruns   uint64
}

// parseFirewallCursor accepts only the epoch and monotonic ID emitted by us.
func parseFirewallCursor(value string) (string, int64, bool) {
	epoch, id, ok := strings.Cut(value, ":")
	cursor, err := strconv.ParseInt(id, 10, 64)
	return epoch, cursor, ok && len(epoch) > 0 && len(epoch) <= 80 && cursor >= 0 && err == nil
}

// poll never falls back to logd. A failed collector is a visible unavailable
// state; otherwise an empty result means the network is quiet.
func (f *fwLogReader) poll(ctx context.Context) []fwEvent {
	limit, after := fwLogBurst, f.cursor
	if !f.started {
		limit, after = f.limit, -1
	}
	batch, err := f.backend.FirewallLogRead(ctx, f.sid, f.generation, after, limit)
	f.reset, f.lost, f.available = false, false, false
	if err != nil {
		if !f.reported {
			f.reported = true
			log.Printf("verso: firewall activity: packet buffer unavailable: %v", err)
		}
		return nil
	}
	f.reported = false
	f.available, f.reset = batch.Available, batch.Reset
	f.lost = batch.Lost || batch.Overruns > f.overruns
	f.overruns = batch.Overruns
	f.generation = batch.Generation
	if batch.Reset {
		f.cursor = 0
	}
	f.refresh(ctx)
	rows := make([]fwEvent, 0, len(batch.Entries))
	for _, entry := range batch.Entries {
		if entry.ID <= f.cursor {
			continue
		}
		f.cursor = entry.ID
		if line, ok := parseFWLogLine(entry.Msg); ok {
			rows = append(rows, f.resolver.event(entry.ID, entry.Time/1000, line))
		}
	}
	f.started = true
	return rows
}

// highestID is the newest record in a tail, or zero for an empty one.
func highestID(entries []openwrt.LogEntry) int64 {
	var highest int64
	for _, entry := range entries {
		if entry.ID > highest {
			highest = entry.ID
		}
	}
	return highest
}

// refresh rebuilds the rule and zone mappings when they have aged out. They
// answer "which rule wrote this prefix" and "which zone owns this device" —
// both configuration, both changing on a person's clock, so a connection reads
// them once and then rarely.
func (f *fwLogReader) refresh(ctx context.Context) {
	if f.resolver != nil && time.Since(f.mapped) < fwLogMapTTL {
		return
	}
	// The attempt is what ages, not only its success: a config that cannot be
	// read must not be asked for once a second for as long as the page is open.
	f.mapped = time.Now()
	config, err := f.backend.UCIConfig(ctx, f.sid, "firewall")
	if err != nil {
		if f.resolver == nil {
			// Nothing to resolve through yet: rows still flow, naming the
			// kernel's own devices and no rules.
			f.resolver = newFWResolver(nil, nil)
			log.Printf("verso: firewall activity: config unreadable, streaming unresolved rows: %v", err)
		}
		return
	}
	ifaces, err := f.backend.NetworkInterfaces(ctx, f.sid)
	if err != nil {
		ifaces = nil // no zone join; a row names the device the kernel named
	}
	f.resolver = newFWResolver(config, ifaces)
}

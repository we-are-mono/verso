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
	// fwLogTail is how much of the log ring one poll asks for. Comfortably more
	// than a second of even a hammered uplink, so nothing is missed between
	// ticks, and small enough to stay a cheap read.
	fwLogTail = 200
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

// streamFirewallLog reads the device's log ring on the sampling clock, keeps
// the netfilter verdicts, resolves each into a row, and pushes it.
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
	if resume, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && resume > 0 {
		reader.cursor, reader.started = resume, true
	}
	send := func() bool {
		rows := reader.poll(r.Context())
		if len(rows) == 0 {
			return true
		}
		payload, err := json.Marshal(map[string]any{"rows": rows})
		if err != nil {
			return true // a formatting slip costs a frame, not the stream
		}
		// The frame's id is the newest record in it, so a reconnect resumes
		// exactly where this one left off.
		_, err = fmt.Fprintf(w, "id: %d\nevent: stream\ndata: %s\n\n", rows[len(rows)-1].ID, payload)
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
	// A read that fails is a quiet second, not a dead page — but a page that
	// stays empty because the session may not read the log looks exactly like a
	// network with nothing to say. Naming the first failure is the difference.
	reported bool
}

// poll reads what the ring has gained since the last one and returns it oldest
// first — the order the shell's client folds into its own newest-on-top view.
// Every failure degrades to "no rows this tick": a log read that fails is a
// quiet second, not a dead page.
func (f *fwLogReader) poll(ctx context.Context) []fwEvent {
	entries, err := f.backend.LogRead(ctx, f.sid, fwLogTail)
	if err != nil {
		if !f.reported {
			f.reported = true
			log.Printf("verso: firewall activity: reading the log ring failed, the listing stays empty: %v", err)
		}
		return nil
	}
	f.reported = false
	f.refresh(ctx)
	// logd's ids count from zero for the life of the daemon. A ring whose
	// newest record is older than the cursor is a log that restarted under the
	// reader, so the cursor is the one thing that must not be trusted.
	if newest := highestID(entries); newest < f.cursor {
		f.cursor = 0
	}
	rows := make([]fwEvent, 0, 8)
	highest := f.cursor
	for _, entry := range entries {
		if entry.ID <= f.cursor {
			continue
		}
		if entry.ID > highest {
			highest = entry.ID
		}
		line, ok := parseFWLogLine(entry.Msg)
		if !ok {
			continue // somebody else's line: the ring carries everything
		}
		rows = append(rows, f.resolver.event(entry.ID, entry.Time/1000, line))
	}
	f.cursor = highest
	// The first poll is a page opening onto history; every later one is the
	// present, and the present is never trimmed from the front.
	if !f.started {
		f.started = true
		if len(rows) > f.limit {
			rows = rows[len(rows)-f.limit:]
		}
		return rows
	}
	if len(rows) > fwLogBurst {
		rows = rows[len(rows)-fwLogBurst:]
	}
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

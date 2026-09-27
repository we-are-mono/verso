// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// errStreamRead is the read failure the degradation cases inject.
var errStreamRead = errors.New("the session may not read that")

// fwLogBackend is a device whose packet buffer holds captured netfilter metadata and
// whose firewall config and interfaces are the ones those lines refer to — the
// whole path from a log record to a row, with no device in it (ADR-003).
func fwLogBackend() fakeBackend {
	return fakeBackend{
		firewallEntries: []openwrt.LogEntry{
			{ID: 4201, Priority: 4, Source: 0, Time: 1788294051000, Msg: fwLineRuleDrop},
			{ID: 4202, Priority: 4, Source: 0, Time: 1788294052000, Msg: fwLineForward},
			{ID: 4203, Priority: 4, Source: 0, Time: 1788294053000, Msg: fwLineZonePol},
		},
		uci:       map[string]map[string]any{"firewall": fwTestConfig()},
		netIfaces: fwTestIfaces(),
	}
}

func decodeStreamRows(t *testing.T, payload string) []fwEvent {
	t.Helper()
	var decoded struct {
		Rows []fwEvent `json:"rows"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("stream JSON: %v\n%s", err, payload)
	}
	return decoded.Rows
}

// openStream opens the source's stream on a signed-in session and returns the
// first frame's data payload.
func openStream(t *testing.T, s *Server, source string) (*http.Response, string) {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/streams/"+source, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /streams/%s: %v", source, err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		return res, ""
	}
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			return res, data
		}
	}
	t.Fatalf("stream ended with no frame: %v", scanner.Err())
	return res, ""
}

// TestFirewallLogStreamCarriesResolvedRows: the whole path — NFLOG records,
// through the netfilter parse, through the config and netifd join, onto the
// wire as structured rows. The lines are the shapes the kernel really writes.
func TestFirewallLogStreamCarriesResolvedRows(t *testing.T) {
	s := newServer(t, fwLogBackend())
	s.eventInterval = 5 * time.Millisecond

	res, payload := openStream(t, s, "firewall-log")
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	rows := decodeStreamRows(t, payload)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want the three packet events: %+v", len(rows), rows)
	}
	first := rows[0]
	if first.ID != 4201 || first.At != 1788294051 {
		t.Errorf("cursor and stamp not carried: %+v", first)
	}
	if first.Verdict != "drop" || first.Rule != "Log-WAN-probes" || first.RuleHref != "/plugins/firewall/?open=cfg11dc81" {
		t.Errorf("rule not resolved: %+v", first)
	}
	if first.From != "wan" || first.Src != "203.0.113.9" || first.ToKind != "router" || first.Port != "445" {
		t.Errorf("packet not resolved: %+v", first)
	}
	if rows[2].Verdict != "reject" || rows[2].Rule != "" {
		t.Errorf("a zone policy has no rule: %+v", rows[2])
	}
	// The wire carries data, never markup: the browser builds the row.
	if strings.Contains(payload, "<") {
		t.Errorf("stream payload must carry no markup:\n%s", payload)
	}
}

// TestFirewallLogStreamAdvancesItsCursor: a record already sent is not sent
// again, so a hammered ring never replays what the page already shows.
func TestFirewallLogStreamAdvancesItsCursor(t *testing.T) {
	backend := fwLogBackend()
	reads := 0
	backend.firewallReads = &reads
	reader := &fwLogReader{backend: backend, sid: "sid", limit: fwLogBacklog}

	first := reader.poll(context.Background())
	if len(first) != 3 {
		t.Fatalf("first poll = %d rows, want 3", len(first))
	}
	if again := reader.poll(context.Background()); len(again) != 0 {
		t.Errorf("second poll replayed %d rows", len(again))
	}
	if reads != 2 {
		t.Errorf("log reads = %d, want one per poll", reads)
	}
	if reader.cursor != 4203 {
		t.Errorf("cursor = %d, want the highest id seen", reader.cursor)
	}
}

// TestFirewallLogStreamTrimsTheBacklogOnly: a page opening onto a full ring is
// given the recent past, not all of it; once open, nothing is trimmed from the
// front until a burst outruns what a frame should carry.
func TestFirewallLogStreamTrimsTheBacklogOnly(t *testing.T) {
	backend := fwLogBackend()
	backend.firewallEntries = nil
	for i := range 40 {
		backend.firewallEntries = append(backend.firewallEntries, openwrt.LogEntry{
			ID: int64(1000 + i), Time: 1788294051000, Msg: fwLineRuleDrop,
		})
	}
	reader := &fwLogReader{backend: backend, sid: "sid", limit: 10}
	first := reader.poll(context.Background())
	if len(first) != 10 {
		t.Fatalf("backlog = %d rows, want the last 10", len(first))
	}
	if first[0].ID != 1030 || first[9].ID != 1039 {
		t.Errorf("backlog is the recent past, oldest first: %d…%d", first[0].ID, first[9].ID)
	}
}

// TestFirewallLogStreamDegradesRatherThanDies: a config the session cannot read
// costs the rows their rule and zone names, never the stream itself.
func TestFirewallLogStreamDegradesRatherThanDies(t *testing.T) {
	backend := fwLogBackend()
	backend.uciReadErr = errStreamRead
	reader := &fwLogReader{backend: backend, sid: "sid", limit: fwLogBacklog}
	rows := reader.poll(context.Background())
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want the verdicts unresolved but present", len(rows))
	}
	if rows[0].Rule != "" || rows[0].Verdict != "" {
		t.Errorf("an unresolvable row claims nothing: %+v", rows[0])
	}
	if rows[0].From != "wan0" {
		t.Errorf("with no zone join a row names the kernel's device: %q", rows[0].From)
	}
	// A failed read returns no rows; the stream reports availability separately.
	quiet := fwLogBackend()
	quiet.firewallErr = errStreamRead
	silent := &fwLogReader{backend: quiet, sid: "sid", limit: fwLogBacklog}
	if rows := silent.poll(context.Background()); rows != nil {
		t.Errorf("a failed log read must yield no rows, got %+v", rows)
	}
}

// TestFirewallLogStreamResumesWhereTheBrowserLeftOff: the browser reconnects a
// dropped stream on its own and says which event it last saw. Resuming there is
// what keeps a reconnect from replaying a backlog the page already has — so the
// frames carry an id, and the id is honoured.
func TestFirewallLogStreamResumesWhereTheBrowserLeftOff(t *testing.T) {
	s := newServer(t, fwLogBackend())
	s.eventInterval = 5 * time.Millisecond
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	read := func(lastEventID string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/streams/firewall-log", nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer res.Body.Close()
		var id, data string
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if v, ok := strings.CutPrefix(line, "id: "); ok {
				id = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				data = v
				break
			}
		}
		if id != "test:4203" {
			t.Errorf("frame id = %q, want the newest record in it", id)
		}
		return data
	}
	if rows := decodeStreamRows(t, read("")); len(rows) != 3 {
		t.Fatalf("a fresh connection gets the backlog, got %d rows", len(rows))
	}
	// Resuming after the second verdict leaves exactly the third to send.
	rows := decodeStreamRows(t, read("test:4202"))
	if len(rows) != 1 || rows[0].ID != 4203 {
		t.Fatalf("resumed connection = %+v, want only the events after 4202", rows)
	}
}

// TestFirewallLogStreamSurvivesALogThatRestarted: the collector generation
// invalidates cursors from a previous helper process.
func TestFirewallLogStreamSurvivesALogThatRestarted(t *testing.T) {
	reader := &fwLogReader{backend: fwLogBackend(), sid: "sid", limit: fwLogBacklog, cursor: 99000, generation: "old-helper", started: true}
	rows := reader.poll(context.Background())
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want the restarted log's verdicts: %+v", len(rows), rows)
	}
	if reader.cursor != 4203 {
		t.Errorf("cursor = %d, want the restarted log's newest id", reader.cursor)
	}
}

// TestUnknownStreamSourceIs404: the source set is closed at the route as well
// as at the table — the two share one answer, so neither can drift.
func TestUnknownStreamSourceIs404(t *testing.T) {
	s := newServer(t, fwLogBackend())
	res, _ := openStream(t, s, "syslog")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("GET /streams/syslog = %d, want 404", res.StatusCode)
	}
}

// TestEveryServedSourceHasAHandler: the closed source set has two halves —
// the names a table may declare, and the code that answers each one. A name the
// widget admits and this package does not answer must be refused, because an
// open stream that never speaks looks exactly like a quiet network.
func TestEveryServedSourceHasAHandler(t *testing.T) {
	s := newServer(t, fwLogBackend())
	if s.streamHandler(widget.StreamSourceFirewallLog) == nil {
		t.Error("the firewall log is a declared source with no handler")
	}
	if s.streamHandler("syslog") != nil {
		t.Error("a source nobody declared must have no handler")
	}
}

// TestStreamRequiresASession: a stream is behind the same gate as every page —
// no cookie, no stream, and the redirect is what closes a browser's EventSource
// for good.
func TestStreamRequiresASession(t *testing.T) {
	s := newServer(t, fwLogBackend())
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(ts.URL + "/streams/firewall-log")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Errorf("unauthenticated stream = %d, want a redirect to the login page", res.StatusCode)
	}
}

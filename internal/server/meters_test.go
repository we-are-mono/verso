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
	"github.com/we-are-mono/verso/internal/sysstat"
)

// fakeStats is the canned local-kernel source for the overview meters.
type fakeStats struct {
	cpu     int
	cpuErr  error
	root    sysstat.Storage
	rootErr error
}

func (f fakeStats) CPUPercent() (int, error)       { return f.cpu, f.cpuErr }
func (f fakeStats) Root() (sysstat.Storage, error) { return f.root, f.rootErr }

// metersBackend has 2 GiB of memory with 0.8 GiB available — 60% full.
func metersBackend() fakeBackend {
	return fakeBackend{si: openwrt.SystemInfo{
		Load:   [3]int64{7864, 0, 0},                                 // 0.12 in the kernel's 65536 fixed point
		Memory: openwrt.Memory{Total: 2 << 30, Available: 858993459}, // 0.8 GiB
	}}
}

func decodeMeters(t *testing.T, payload string) []meterReading {
	t.Helper()
	var decoded struct {
		Meters []meterReading `json:"meters"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("meters JSON: %v\n%s", err, payload)
	}
	return decoded.Meters
}

// TestOverviewEventsStream: the SSE endpoint holds the connection open and
// pushes `meters` events on the sampling clock — two in a row proves the
// stream streams, and the payload is the readings JSON.
func TestOverviewEventsStream(t *testing.T) {
	s := newServer(t, metersBackend())
	s.stats = fakeStats{cpu: 18, root: sysstat.Storage{Used: 23 << 30, Free: 9 << 30}}
	s.eventInterval = 5 * time.Millisecond

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token, err := s.sessions.Create("test-sid", "root")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /overview/events: %v", err)
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// The stream carries several event types now (meters, interfaces, wan);
	// collect the two meters payloads, tracking the current event.
	var payloads []string
	var event string
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() && len(payloads) < 2 {
		line := scanner.Text()
		if ev, ok := strings.CutPrefix(line, "event: "); ok {
			event = ev
			continue
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			if event == "meters" {
				payloads = append(payloads, data)
			}
			continue
		}
		if line != "" {
			t.Fatalf("unexpected stream line %q", line)
		}
	}
	if len(payloads) < 2 {
		t.Fatalf("stream ended after %d meters event(s): %v", len(payloads), scanner.Err())
	}
	meters := decodeMeters(t, payloads[0])
	if len(meters) != 4 || meters[0].Name != "sys-load" {
		t.Fatalf("first event payload = %+v", meters)
	}
}

// TestIndexWithoutMetersStillRenders: the overview renders (and answers 200)
// even with every live source down — its System panel is hardcoded for now, so
// the page never depends on the meter sources.
func TestIndexWithoutMetersStillRenders(t *testing.T) {
	s := newServer(t, fakeBackend{err: errors.New("bus down")})
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Your network is") {
		t.Error("GET /: overview should render regardless of the meter sources")
	}
}

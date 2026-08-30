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

// TestMeterReadings: the builder assembles the three box-health readings —
// memory from system info, storage and CPU from the local kernel — with the
// fill, band, and human detail the donuts render from.
func TestMeterReadings(t *testing.T) {
	s := newServer(t, metersBackend())
	s.stats = fakeStats{cpu: 95, root: sysstat.Storage{Used: 23 << 30, Free: 9 << 30}}

	meters := s.meterReadings(context.Background(), "test-sid")
	if len(meters) != 3 {
		t.Fatalf("got %d meters, want 3: %+v", len(meters), meters)
	}

	mem := meters[0]
	if mem.Name != "memory" || mem.Value != "1.2" || mem.Unit != "GB" || mem.Fill != 60 ||
		mem.Band != "good" || mem.Detail != "0.8 GB free" {
		t.Errorf("memory reading = %+v", mem)
	}
	st := meters[1]
	if st.Name != "storage" || st.Value != "23" || st.Fill != 71 || st.Detail != "9 GB free" {
		t.Errorf("storage reading = %+v", st)
	}
	cpu := meters[2]
	if cpu.Name != "cpu" || cpu.Value != "95" || cpu.Unit != "%" || cpu.Fill != 95 ||
		cpu.Band != "danger" || cpu.Detail != "load 0.12" {
		t.Errorf("cpu reading = %+v", cpu)
	}
}

// TestMetersDegrade: a failed source drops its reading — the others still
// arrive, and every source down means an empty set, not an error.
func TestMetersDegrade(t *testing.T) {
	s := newServer(t, fakeBackend{err: errors.New("bus down")})
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}
	if meters := s.meterReadings(context.Background(), "test-sid"); len(meters) != 0 {
		t.Fatalf("all sources down: got %d meters, want 0", len(meters))
	}

	// Only the local kernel down: memory still reports.
	s = newServer(t, metersBackend())
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}
	meters := s.meterReadings(context.Background(), "test-sid")
	if len(meters) != 1 || meters[0].Name != "memory" {
		t.Fatalf("kernel down: got %+v, want just memory", meters)
	}
}

// TestMeterReadingsIncludeSpeed: with the uplink up, the speed donut leads —
// download since the last observation in the centre, upload in the detail,
// fill as the share of the negotiated link, banded "info" (a rate, not a
// getting-full). The tracker's clock and cold-start beat are injected.
func TestMeterReadingsIncludeSpeed(t *testing.T) {
	be := metersBackend()
	be.wan = openwrt.WANState{Up: true, Device: "wan0"}
	stats := []openwrt.DeviceStats{
		{SpeedMbps: 1000, RxBytes: 0, TxBytes: 0},
		{SpeedMbps: 1000, RxBytes: 3_750_000, TxBytes: 375_000}, // +30 / +3 Mbit over 1s
	}
	be.devStats = &stats
	s := newServer(t, be)
	tick := time.Unix(1000, 0)
	s.wan = &wanRate{
		now:  func() time.Time { now := tick; tick = tick.Add(time.Second); return now },
		wait: func() {},
	}

	meters := s.meterReadings(context.Background(), "test-sid")
	if len(meters) != 4 {
		t.Fatalf("got %d meters, want 4: %+v", len(meters), meters)
	}
	sp := meters[0]
	if sp.Name != "speed" || sp.Value != "30" || sp.Unit != "Mbps" || sp.Fill != 3 ||
		sp.Band != "info" || sp.Detail != "3 Mbps up" {
		t.Errorf("speed reading = %+v", sp)
	}
}

// TestSpeedAbsentWhenWANDown: a downed uplink drops the speed donut — the
// hero owns the internet-down story, not a zeroed gauge.
func TestSpeedAbsentWhenWANDown(t *testing.T) {
	be := metersBackend()
	be.wan = openwrt.WANState{Up: false, Device: "wan0"}
	for _, m := range newServer(t, be).meterReadings(context.Background(), "test-sid") {
		if m.Name == "speed" {
			t.Fatalf("speed reported with wan down: %+v", m)
		}
	}
}

// TestWANRateReseedsOnCounterReset: counters that move backwards (an
// interface bounce) reseed instead of reporting a negative rate.
func TestWANRateReseedsOnCounterReset(t *testing.T) {
	tick := time.Unix(0, 0)
	w := &wanRate{now: func() time.Time { now := tick; tick = tick.Add(time.Second); return now }}
	if _, _, ok := w.observe(1000, 1000); ok {
		t.Fatal("first observation cannot have a rate")
	}
	if _, _, ok := w.observe(500, 500); ok {
		t.Fatal("backwards counters must reseed, not report")
	}
	down, up, ok := w.observe(1500, 1000)
	if !ok || down != 8000 || up != 4000 {
		t.Fatalf("post-reseed rate = %v/%v ok=%v, want 8000/4000 true", down, up, ok)
	}
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

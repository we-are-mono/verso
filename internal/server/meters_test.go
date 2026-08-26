// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"net/http"
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

func (f fakeStats) CPUPercent() (int, error)        { return f.cpu, f.cpuErr }
func (f fakeStats) Root() (sysstat.Storage, error)  { return f.root, f.rootErr }

// metersBackend has 2 GiB of memory with 0.8 GiB available — 60% full.
func metersBackend() fakeBackend {
	return fakeBackend{si: openwrt.SystemInfo{
		Load:   [3]int64{7864, 0, 0}, // 0.12 in the kernel's 65536 fixed point
		Memory: openwrt.Memory{Total: 2 << 30, Available: 858993459}, // 0.8 GiB
	}}
}

func decodeMeters(t *testing.T, body string) []meterReading {
	t.Helper()
	var payload struct {
		Meters []meterReading `json:"meters"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("meters JSON: %v\n%s", err, body)
	}
	return payload.Meters
}

// TestMetersJSON: the live endpoint serves the three box-health readings —
// memory from system info, storage and CPU from the local kernel — with the
// fill, band, and human detail the donuts render from.
func TestMetersJSON(t *testing.T) {
	s := newServer(t, metersBackend())
	s.stats = fakeStats{cpu: 95, root: sysstat.Storage{Used: 23 << 30, Free: 9 << 30}}

	rec := get(t, s, "/overview/meters")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /overview/meters: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	meters := decodeMeters(t, rec.Body.String())
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

// TestMetersDegrade: a failed source drops its reading — never a 500, and the
// others still arrive.
func TestMetersDegrade(t *testing.T) {
	s := newServer(t, fakeBackend{err: errors.New("bus down")})
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}

	rec := get(t, s, "/overview/meters")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /overview/meters: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if meters := decodeMeters(t, rec.Body.String()); len(meters) != 0 {
		t.Fatalf("all sources down: got %d meters, want 0", len(meters))
	}

	// Only the local kernel down: memory still reports.
	s = newServer(t, metersBackend())
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}
	meters := decodeMeters(t, get(t, s, "/overview/meters").Body.String())
	if len(meters) != 1 || meters[0].Name != "memory" {
		t.Fatalf("kernel down: got %+v, want just memory", meters)
	}
}

// TestMetersJSONIncludesSpeed: with the uplink up, the speed donut leads —
// download since the last observation in the centre, upload in the detail,
// fill as the share of the negotiated link, banded "info" (a rate, not a
// getting-full). The tracker's clock and cold-start beat are injected.
func TestMetersJSONIncludesSpeed(t *testing.T) {
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

	meters := decodeMeters(t, get(t, s, "/overview/meters").Body.String())
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
	meters := decodeMeters(t, get(t, newServer(t, be), "/overview/meters").Body.String())
	for _, m := range meters {
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

// TestIndexRendersLiveMeters: the homepage leads with the three named donuts,
// so the poller has handles to stream into.
func TestIndexRendersLiveMeters(t *testing.T) {
	rec := get(t, newServer(t, metersBackend()), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-verso-meter="memory"`, `data-verso-meter="storage"`, `data-verso-meter="cpu"`,
		"Memory", "Storage", "Processor",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
}

// TestIndexWithoutMetersStillRenders: every source down drops the donut row
// entirely (no empty grid), and the page still answers.
func TestIndexWithoutMetersStillRenders(t *testing.T) {
	s := newServer(t, fakeBackend{err: errors.New("bus down")})
	s.stats = fakeStats{cpuErr: errors.New("no proc"), rootErr: errors.New("no statfs")}
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "data-verso-meter") {
		t.Error("GET /: donuts rendered with every source down")
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
)

func historyAt(step *time.Time) *trafficHistory {
	return newTrafficHistory(func() time.Time { return *step })
}

// TestTrafficHistory: totals deltas become per-address rates, one point a
// second; a device's addresses sum, in Mbps, oldest first.
func TestTrafficHistory(t *testing.T) {
	now := time.Unix(0, 0)
	h := historyAt(&now)

	h.Observe(map[string]sysstat.DeviceTraffic{
		"192.168.77.102": {RxBytes: 0, TxBytes: 0},
		"fd42::66":       {RxBytes: 0, TxBytes: 0},
	})
	now = now.Add(time.Second)
	h.Observe(map[string]sysstat.DeviceTraffic{
		"192.168.77.102": {RxBytes: 1_250_000, TxBytes: 125_000}, // 10 / 1 Mbit over 1s
		"fd42::66":       {RxBytes: 250_000, TxBytes: 0},         // 2 Mbit
	})
	now = now.Add(time.Second)
	h.Observe(map[string]sysstat.DeviceTraffic{
		"192.168.77.102": {RxBytes: 1_250_000, TxBytes: 125_000}, // quiet second
		"fd42::66":       {RxBytes: 250_000, TxBytes: 0},
	})

	down, up := h.Series([]string{"192.168.77.102", "fd42::66"})
	if len(down) != trafficPoints || len(up) != trafficPoints {
		t.Fatalf("series lengths = %d/%d, want the full minute", len(down), len(up))
	}
	if down[trafficPoints-2] != 12 || up[trafficPoints-2] != 1 {
		t.Errorf("first observed point = %v/%v Mbps, want 12/1", down[trafficPoints-2], up[trafficPoints-2])
	}
	if down[trafficPoints-1] != 0 || up[trafficPoints-1] != 0 {
		t.Errorf("quiet point = %v/%v, want zeros", down[trafficPoints-1], up[trafficPoints-1])
	}
	if down[0] != 0 { // the not-yet-observed left of the window stays quiet
		t.Errorf("padding = %v, want 0", down[0])
	}
}

// TestTrafficHistoryDedupes: overlapping viewers observing within the same
// second don't double the tempo.
func TestTrafficHistoryDedupes(t *testing.T) {
	now := time.Unix(0, 0)
	h := historyAt(&now)
	h.Observe(map[string]sysstat.DeviceTraffic{"a": {}})
	now = now.Add(200 * time.Millisecond)
	h.Observe(map[string]sysstat.DeviceTraffic{"a": {RxBytes: 1 << 20}})
	if down, _ := h.Series([]string{"a"}); len(down) != 0 {
		t.Fatalf("sub-second observation must be dropped, got %v", down)
	}
}

// TestTrafficHistoryRing: the ring holds the last minute and keeps order.
func TestTrafficHistoryRing(t *testing.T) {
	now := time.Unix(0, 0)
	h := historyAt(&now)
	total := int64(0)
	h.Observe(map[string]sysstat.DeviceTraffic{"a": {}})
	for i := 0; i < trafficPoints+10; i++ {
		now = now.Add(time.Second)
		total += int64(i) * 125_000 // i Mbit that second
		h.Observe(map[string]sysstat.DeviceTraffic{"a": {RxBytes: total}})
	}
	down, _ := h.Series([]string{"a"})
	if len(down) != trafficPoints {
		t.Fatalf("series length = %d, want %d", len(down), trafficPoints)
	}
	if down[len(down)-1] != float64(trafficPoints+9) {
		t.Errorf("newest point = %v, want %d", down[len(down)-1], trafficPoints+9)
	}
	if down[0] != float64(10) {
		t.Errorf("oldest kept point = %v, want 10", down[0])
	}
}

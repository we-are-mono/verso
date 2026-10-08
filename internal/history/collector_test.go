// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"testing"
	"time"
)

// TestCollectorFoldsReadingsIntoMinutes: every reading taken within a minute
// counts toward that minute's average and peak, and the minute reaches the
// store once the next one has begun.
func TestCollectorFoldsReadingsIntoMinutes(t *testing.T) {
	store, _ := open(t)
	readings := map[string][]float64{}
	c := NewCollector(store, func(time.Time) map[string][]float64 { return readings })

	readings = map[string][]float64{"cpu": {10, 20}, "iface.eth0.rx": {100}}
	c.Tick(minute(0).Add(10 * time.Second))
	readings = map[string][]float64{"cpu": {60}, "iface.eth0.rx": {300}}
	c.Tick(minute(0).Add(50 * time.Second))
	readings = map[string][]float64{"cpu": {5}}
	c.Tick(minute(1).Add(10 * time.Second))
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	cpu, err := store.Points(ctx, "cpu", Minute, minute(0), minute(2))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Point{Series: "cpu", Time: minute(0), Average: 30, Peak: 60}); len(cpu) != 1 || cpu[0] != want {
		t.Fatalf("cpu = %+v, want only the finished minute, %+v", cpu, want)
	}
	rx, err := store.Points(ctx, "iface.eth0.rx", Minute, minute(0), minute(2))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Point{Series: "iface.eth0.rx", Time: minute(0), Average: 200, Peak: 300}); len(rx) != 1 || rx[0] != want {
		t.Fatalf("rx = %+v, want %+v", rx, want)
	}
}

// TestCollectorReadingAtTheMinuteBelongsToTheMinuteBefore: a tick on the
// minute carries the readings of the seconds before it.
func TestCollectorReadingAtTheMinuteBelongsToTheMinuteBefore(t *testing.T) {
	store, _ := open(t)
	value := 0.0
	c := NewCollector(store, func(time.Time) map[string][]float64 {
		return map[string][]float64{"cpu": {value}}
	})
	value = 40
	c.Tick(minute(1))
	value = 2
	c.Tick(minute(1).Add(10 * time.Second))
	c.Tick(minute(2).Add(10 * time.Second))
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Minute, minute(0), minute(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Time != minute(0) || got[0].Average != 40 || got[1].Time != minute(1) {
		t.Fatalf("points = %+v, want minute 0 holding the reading taken on minute 1", got)
	}
}

// TestCollectorLeavesAGapForAMinuteWithoutReadings: nothing is invented for a
// minute the shell did not see.
func TestCollectorLeavesAGapForAMinuteWithoutReadings(t *testing.T) {
	store, _ := open(t)
	c := NewCollector(store, func(time.Time) map[string][]float64 {
		return map[string][]float64{"cpu": {1}}
	})
	c.Tick(minute(0).Add(30 * time.Second))
	c.Tick(minute(5).Add(30 * time.Second))
	c.Tick(minute(6).Add(30 * time.Second))
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Minute, minute(0), minute(7))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Time != minute(0) || got[1].Time != minute(5) {
		t.Fatalf("points = %+v, want minutes 0 and 5 only", got)
	}
}

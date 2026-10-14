// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/units"
)

type fakeTelemetry struct {
	snapshot telemetry.Snapshot
	calls    *int
}

func (f fakeTelemetry) Snapshot(context.Context) (telemetry.Snapshot, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	return f.snapshot, nil
}

func TestTelemetrySnapshotIsSharedWithinTick(t *testing.T) {
	calls := 0
	now := time.Now()
	s := &Server{telemetry: fakeTelemetry{calls: &calls, snapshot: telemetry.Snapshot{
		TimestampMS: uint64(now.UnixMilli()),
	}}}
	if _, ok := s.telemetrySnapshot(context.Background()); !ok {
		t.Fatal("fresh snapshot unavailable")
	}
	if _, ok := s.telemetrySnapshot(context.Background()); !ok {
		t.Fatal("cached snapshot unavailable")
	}
	if calls != 1 {
		t.Fatalf("reads = %d, want 1", calls)
	}
}

func TestTelemetryInterfaceReadings(t *testing.T) {
	slovenian := func(s string) string {
		if s == "." {
			return ","
		}
		return s
	}
	readings := telemetryInterfaceReadings(telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth4", Operstate: "up", History: []telemetry.Point{{RxBPS: 13_867_160, TxBPS: 141_304}}},
		{Name: "tun0", Operstate: "unknown", History: []telemetry.Point{{}}},
	}}, units.For(slovenian))
	if len(readings) != 2 || readings[0].Name != "eth4" || readings[0].State != "Up" ||
		readings[0].Variant != "success" || readings[0].RxRate != "14 Mb/s" || readings[0].TxRate != "0,1 Mb/s" {
		t.Fatalf("eth4 reading = %+v", readings)
	}
	if readings[1].State != "Unknown" || readings[1].Variant != "neutral" || readings[1].RxRate != "0 Mb/s" {
		t.Fatalf("tun0 reading = %+v", readings[1])
	}
}

// TestInterfaceReadingsLightThePanel: each reading carries what a port's LEDs
// and label show — its link, its activity level from the packets it moved
// and its speed — so the stream relights the panel as it relists the table.
func TestInterfaceReadingsLightThePanel(t *testing.T) {
	readings := telemetryInterfaceReadings(telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth0", Physical: true, Operstate: "up", SpeedMbps: 1000, History: []telemetry.Point{{RxPPS: 30_000, TxPPS: 20_000}}},
		{Name: "eth1", Physical: true, Operstate: "down", SpeedMbps: -1, History: []telemetry.Point{{}}},
		{Name: "eth2", Physical: true, Operstate: "up", SpeedMbps: 10000, History: []telemetry.Point{{RxPPS: 4}}},
	}}, units.For(func(s string) string { return s }))
	for i, want := range []interfaceReading{
		{Name: "eth0", Linked: true, Activity: 3, Speed: "1G"},
		{Name: "eth1", Linked: false, Activity: 0, Speed: "—"},
		{Name: "eth2", Linked: true, Activity: 1, Speed: "10G"},
	} {
		got := readings[i]
		if got.Name != want.Name || got.Linked != want.Linked || got.Activity != want.Activity || got.Speed != want.Speed {
			t.Errorf("reading %d = %+v, want %+v", i, got, want)
		}
	}
}

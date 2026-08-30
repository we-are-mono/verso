// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/telemetry"
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
		Version: 1, TimestampMS: uint64(now.UnixMilli()),
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
	readings := telemetryInterfaceReadings(telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth4", Operstate: "up", History: []telemetry.Point{{RxBPS: 13_867_160, TxBPS: 141_304}}},
		{Name: "tun0", Operstate: "unknown", History: []telemetry.Point{{}}},
	}})
	if len(readings) != 2 || readings[0].Name != "eth4" || readings[0].State != "Up" ||
		readings[0].Variant != "success" || readings[0].RxRate != "13.9 Mbps" || readings[0].TxRate != "141.3 Kbps" {
		t.Fatalf("eth4 reading = %+v", readings)
	}
	if readings[1].State != "Unknown" || readings[1].Variant != "neutral" {
		t.Fatalf("tun0 reading = %+v", readings[1])
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/telemetry"
)

// TestInterfaceRatesHandsOverEachSecondOnce: each per-second rate reaches the
// history once, however the ticks fall. Loopback is not recorded, nor a device
// that is down: an idle tunnel placeholder or a port without link is a gap,
// not a line of zeros.
func TestInterfaceRatesHandsOverEachSecondOnce(t *testing.T) {
	at := func(s int64) uint64 { return uint64(s * 1000) }
	snapshot := telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth0", History: []telemetry.Point{
			{TimestampMS: at(1), RxBPS: 10, TxBPS: 1},
			{TimestampMS: at(2), RxBPS: 20, TxBPS: 2},
		}},
		{Name: "lo", History: []telemetry.Point{{TimestampMS: at(2), RxBPS: 5}}},
		{Name: "sit0", Operstate: "down", History: []telemetry.Point{{TimestampMS: at(2)}}},
	}}
	fake := &fakeTelemetry{snapshot: snapshot}
	source := interfaceRates(fake)

	got := source(time.Unix(2, 0))
	if !slices.Equal(got["iface.eth0.rx"], []float64{10, 20}) || !slices.Equal(got["iface.eth0.tx"], []float64{1, 2}) {
		t.Fatalf("first reading = %v, want both seconds of eth0", got)
	}
	if _, ok := got["iface.lo.rx"]; ok {
		t.Fatal("loopback was recorded")
	}
	if _, ok := got["iface.sit0.rx"]; ok {
		t.Fatal("a device that is down was recorded")
	}

	fake.snapshot.Interfaces[0].History = []telemetry.Point{
		{TimestampMS: at(2), RxBPS: 20, TxBPS: 2},
		{TimestampMS: at(3), RxBPS: 30, TxBPS: 3},
	}
	got = source(time.Unix(3, 0))
	if !slices.Equal(got["iface.eth0.rx"], []float64{30}) {
		t.Fatalf("second reading = %v, want only the new second", got)
	}
}

// TestMemoryInUseIsTotalLessAvailable: memory the kernel could hand out at
// once (page cache included) is not in use.
func TestMemoryInUseIsTotalLessAvailable(t *testing.T) {
	meminfo := []byte("MemTotal:        1000 kB\nMemFree:          100 kB\nMemAvailable:     600 kB\n")
	got, err := memoryInUse(meminfo)
	if err != nil || got != 400*1024 {
		t.Fatalf("memoryInUse = %v, %v; want 409600 bytes", got, err)
	}
	if _, err := memoryInUse([]byte("MemTotal: 1000 kB\n")); err == nil {
		t.Fatal("a meminfo without MemAvailable read as a value")
	}
}

// TestTemperatureSeriesNameEveryAmbiguousSensorByItsChip: two drives that both
// call a sensor "Composite" are told apart by chip, whichever is read first, so
// a series always means the same sensor; a faulted reading is a gap.
func TestTemperatureSeriesNameEveryAmbiguousSensorByItsChip(t *testing.T) {
	got := temperatureSeries([]sensors.TempReading{
		{Name: "Composite", Source: "nvme · 0f:00", MilliC: 46000},
		{Name: "CPU", Source: "k10temp", MilliC: 80500},
		{Name: "Composite", Source: "nvme · 0e:00", MilliC: 48000},
		{Name: "Remote", Source: "lm75", Fault: true},
	})
	want := map[string][]float64{
		"temp.Composite.nvme · 0f:00": {46},
		"temp.Composite.nvme · 0e:00": {48},
		"temp.CPU":                    {80.5},
	}
	if len(got) != len(want) {
		t.Fatalf("series = %v, want %v", got, want)
	}
	for series, values := range want {
		if !slices.Equal(got[series], values) {
			t.Fatalf("series = %v, want %v", got, want)
		}
	}
}

// TestGaugeSkipsWhatCannotBeRead: a reading that fails leaves a gap, never a
// zero.
func TestGaugeSkipsWhatCannotBeRead(t *testing.T) {
	source := gauges(map[string]func() (float64, error){
		"cpu":       func() (float64, error) { return 12.5, nil },
		"conntrack": func() (float64, error) { return 0, errors.New("absent") },
	})
	got := source(time.Unix(0, 0))
	if len(got) != 1 || !slices.Equal(got["cpu"], []float64{12.5}) {
		t.Fatalf("gauges = %v, want only cpu", got)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
)

// countingSampler is a sampler on a clock the test moves, counting its reads.
func countingSampler(now *time.Time) (*sensorSampler, *atomic.Int32) {
	var reads atomic.Int32
	return &sensorSampler{
		read: func() sensors.Inventory { reads.Add(1); time.Sleep(time.Millisecond); return sensors.Inventory{} },
		now:  func() time.Time { return *now },
	}, &reads
}

// One read serves everyone for five seconds, and concurrent callers on a stale
// read wait for one read rather than each starting their own.
func TestSensorSamplerReadsOncePerFreshness(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s, reads := countingSampler(&now)
	s.Read()
	now = now.Add(4 * time.Second)
	if s.Read(); reads.Load() != 1 {
		t.Fatalf("within five seconds: %d reads, want 1", reads.Load())
	}
	now = now.Add(time.Second)
	if s.Read(); reads.Load() != 2 {
		t.Fatalf("after five seconds: %d reads, want 2", reads.Load())
	}

	now = now.Add(time.Minute)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { s.Read() })
	}
	wg.Wait()
	if reads.Load() != 3 {
		t.Fatalf("eight concurrent callers on a stale read: %d reads, want 3", reads.Load())
	}
}

// The card gets the profile's readings in its order, written as the Hardware
// page writes them: a faulted temperature and a fan that does not turn read
// "—", a stopped fan is a warning, a warm reading carries its tone, and the
// CPU's value is the closed card's line.
func TestHardwareView(t *testing.T) {
	inv := sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "Memory", MilliC: 86_400, Level: "warn", Entry: 2},
			{Name: "cluster", MilliC: 48_400, Level: "nominal", CPU: true, Entry: 1},
			{Name: "unprofiled", MilliC: 30_000},
			{Name: "SFP+ modules", Fault: true, Entry: 3},
		},
		Powers: []sensors.PowerReading{{Name: "USB Power Delivery", MicroW: 9_800_000, Entry: 1}},
		Fans: []sensors.FanReading{
			{Name: "System Fan 1", RPM: 2622, State: "running", Entry: 1},
			{Name: "System Fan 2", State: "absent", Entry: 2},
			{Name: "System Fan 3", State: "fault", Entry: 3},
		},
	}
	got := hardwareView(inv)
	want := &widget.OverviewSensors{
		CPU: "48 °C",
		Thermal: []widget.OverviewSensor{
			{Key: "hw-thermal-1", Name: "cluster", Value: "48 °C", Tone: "success"},
			{Key: "hw-thermal-2", Name: "Memory", Value: "86 °C", Tone: "warning"},
			{Key: "hw-thermal-3", Name: "SFP+ modules", Value: "—", Tone: "neutral"},
		},
		Power: []widget.OverviewSensor{{Key: "hw-power-1", Name: "USB Power Delivery", Value: "9.8 W"}},
		Fans: []widget.OverviewSensor{
			{Key: "hw-fans-1", Name: "System Fan 1", Value: "2,622 rpm"},
			{Key: "hw-fans-2", Name: "System Fan 2", Value: "—"},
			{Key: "hw-fans-3", Name: "System Fan 3", Value: "—", Tone: "warning"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("view = %+v\nwant %+v", got, want)
	}
	if rows := hardwareRows(got); rows["hw-thermal-2"] != "86 °C" || rows["hw-fans-2"] != "—" || len(rows) != 7 {
		t.Errorf("rows = %v", rows)
	}
	if hardwareView(sensors.Inventory{Temps: []sensors.TempReading{{Name: "unprofiled"}}}) != nil {
		t.Error("no profiled reading answered: want no view")
	}
}

// profiledServer is a stream server on a board whose profile lists sensors,
// their reads counted.
func profiledServer(t *testing.T) (*Server, *atomic.Int32) {
	srv := streamServer(t)
	now := time.Now()
	sampler, reads := countingSampler(&now)
	sampler.read = func() sensors.Inventory {
		reads.Add(1)
		return sensors.Inventory{
			Temps: []sensors.TempReading{{Name: "cluster", MilliC: 52_000, Level: "warn", CPU: true, Entry: 1}},
			Fans:  []sensors.FanReading{{Name: "System Fan 1", RPM: 2622, State: "running", Entry: 1}},
		}
	}
	srv.layouts, srv.boardSensors = cookieLayout{hardware: true}, sampler
	return srv, reads
}

// On a profiled board the Overview opens with the Hardware card, its rows
// keyed for the stream and its header naming the warm reading.
func TestOverviewRendersTheHardwareCard(t *testing.T) {
	srv, _ := profiledServer(t)
	body := get(t, srv, "/").Body.String()
	for _, want := range []string{`data-overview-card="hardware"`, `data-verso-prop="hw-thermal-1"`, "52 °C", "cluster is hot", `href="/system/hardware"`} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
}

// The sensors frame rides the stream while Hardware is shown, open or closed;
// hidden, the frame is not sent and the board's sensors are not read.
func TestOverviewStreamSendsSensorsWhileHardwareIsShown(t *testing.T) {
	for _, tc := range []struct {
		name, layout string
		want         bool
	}{
		{"default", "", true},
		{"Hardware closed", "1.internet-ports-tunnels.traffic-resources-system-hardware.hardware.", true},
		{"Hardware hidden", "1.internet-ports-tunnels.traffic-resources-system..hardware", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, reads := profiledServer(t)
			frames := firstFrames(t, srv, tc.layout)
			payload, got := frames["sensors"]
			if got != tc.want || (reads.Load() > 0) != tc.want {
				t.Fatalf("sensors frame sent = %v after %d reads, want %v", got, reads.Load(), tc.want)
			}
			if tc.want && (!strings.Contains(payload, `"hw-thermal-1":"52 °C"`) || !strings.Contains(payload, `"hw-fans-1":"2,622 rpm"`)) {
				t.Errorf("sensors frame = %s", payload)
			}
			if tc.want && !strings.Contains(frames["overview"], "cluster is hot") {
				t.Errorf("the overview frame carries Hardware's head: %s", frames["overview"])
			}
		})
	}
}

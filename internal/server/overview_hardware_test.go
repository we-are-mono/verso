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

// The card gets the board's headlines, written as the Hardware page writes
// them: the CPU, the main rail's draw with its voltage and current, and the
// main fan. Only a CPU past its limit or a stopped fan carries a tone.
func TestHardwareView(t *testing.T) {
	inv := sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "Memory", MilliC: 86_400, Level: "warn"},
			{Name: "cluster", MilliC: 48_400, Level: "nominal", CPU: true},
		},
		Powers: []sensors.PowerReading{
			{Name: "1V Core PSU", MicroW: 3_100_000},
			{Name: "USB Power Delivery", MicroW: 9_800_000, MilliV: 20_100, HasV: true, MilliA: 490, HasA: true, Main: true},
		},
		Fans: []sensors.FanReading{
			{Name: "System Fan 1", RPM: 2622, State: "running", Main: true},
			{Name: "System Fan 2", State: "absent"},
		},
	}
	got := hardwareView(inv)
	want := &widget.OverviewSensors{
		CPU:   &widget.OverviewSensor{Key: "hw-cpu", Name: "cluster", Value: "48 °C"},
		Power: &widget.OverviewSensor{Key: "hw-power", Name: "USB Power Delivery", Value: "9.8 W · 20.10 V · 0.49 A"},
		Fan:   &widget.OverviewSensor{Key: "hw-fan", Name: "System Fan 1", Value: "2,622 rpm"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("view = %+v\nwant %+v", got, want)
	}
	if rows := hardwareRows(got); rows["hw-power"] != "9.8 W · 20.10 V · 0.49 A" || len(rows) != 3 {
		t.Errorf("rows = %v", rows)
	}

	inv.Temps[1].Level, inv.Fans[0].State, inv.Fans[0].RPM = "critical", "fault", 0
	inv.Powers[1].HasA = false
	hot := hardwareView(inv)
	if hot.CPU.Tone != "danger" || hot.Fan.Tone != "warning" || hot.Fan.Value != "—" || hot.Power.Value != "9.8 W · 20.10 V" {
		t.Errorf("out of band = %+v %+v %+v", hot.CPU, hot.Fan, hot.Power)
	}
	if hardwareView(sensors.Inventory{Powers: []sensors.PowerReading{{Name: "1V Core PSU"}}}) != nil {
		t.Error("no headline answered: want no view")
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
			Temps: []sensors.TempReading{{Name: "cluster", MilliC: 52_000, Level: "warn", CPU: true}},
			Fans:  []sensors.FanReading{{Name: "System Fan 1", RPM: 2622, State: "running", Main: true}},
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
	for _, want := range []string{`data-overview-card="hardware"`, `data-verso-prop="hw-cpu"`, "52 °C", "cluster is hot", `href="/system/hardware"`} {
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
			if tc.want && (!strings.Contains(payload, `"hw-cpu":"52 °C"`) || !strings.Contains(payload, `"hw-fan":"2,622 rpm"`)) {
				t.Errorf("sensors frame = %s", payload)
			}
			if tc.want && !strings.Contains(frames["overview"], "cluster is hot") {
				t.Errorf("the overview frame carries Hardware's head: %s", frames["overview"])
			}
		})
	}
}

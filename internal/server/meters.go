// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

// The overview meters (the donuts): the box-health glance — speed, memory,
// storage, processor — as live readings. One builder feeds both faces: the
// page render composes meter widgets from it, and the overview stream
// (events.go) pushes the same readings into the rendered rings. A source that
// fails is omitted (and logged), never a 500 — the glance degrades the way
// the status table does.

// statSource is the local-machine seam the meters read — CPU busy share and
// root-filesystem fullness; sysstat.Sampler is the real one, tests fake it.
type statSource interface {
	CPUPercent() (int, error)
	Root() (sysstat.Storage, error)
}

// meterReading is one donut's worth of truth, shaped for both the widget and
// the live-update JSON (the band rides along so the client never re-derives
// the colour rule).
type meterReading struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Unit    string `json:"unit"`
	Fill    int    `json:"fill"`
	Band    string `json:"band"`
	Detail  string `json:"detail"`
	Variant string `json:"-"`              // widget variant ("info" for a rate); the JSON carries its band instead
	Role    string `json:"role,omitempty"` // decorative bar accent (sky|violet|emerald|amber); the client keeps it instead of a health band
}

// wanRate turns the wan device's byte counters into a throughput reading: the
// delta between the previous observation and this one. One tracker per server
// — the counters are box truth, so whichever session polls advances it.
type wanRate struct {
	now  func() time.Time
	wait func() // the beat for the cold-start double read

	mu     sync.Mutex
	rx, tx int64
	at     time.Time
	seeded bool
}

// observe folds in fresh counters and returns the rates since the previous
// observation, in bits per second. Not ok until a previous observation
// exists; a counter that moved backwards (interface bounce) reseeds.
func (w *wanRate) observe(rx, tx int64) (downBps, upBps float64, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	dt := now.Sub(w.at).Seconds()
	drx, dtx := rx-w.rx, tx-w.tx
	seeded := w.seeded
	w.rx, w.tx, w.at, w.seeded = rx, tx, now, true
	if !seeded || dt <= 0 || drx < 0 || dtx < 0 {
		return 0, 0, false
	}
	return float64(drx) * 8 / dt, float64(dtx) * 8 / dt, true
}

// speedReading is the wan throughput donut: download in the centre, upload in
// the detail, the ring the share of the negotiated link ("info" — a rate has
// no getting-full). Absent when the uplink is down or unreadable: the
// internet story is the hero's to tell, not a zeroed gauge's.
func (s *Server) speedReading(ctx context.Context, sid string) (meterReading, bool) {
	ws, err := s.backend.WANStatus(ctx, sid)
	if err != nil {
		log.Printf("verso: meters: wan status unavailable: %v", err)
		return meterReading{}, false
	}
	if !ws.Up || ws.Device == "" {
		return meterReading{}, false
	}
	st, err := s.backend.DeviceStats(ctx, sid, ws.Device)
	if err != nil {
		log.Printf("verso: meters: wan device stats unavailable: %v", err)
		return meterReading{}, false
	}
	down, up, ok := s.wan.observe(st.RxBytes, st.TxBytes)
	if !ok { // no previous observation — read again a beat later, once
		s.wan.wait()
		st2, err := s.backend.DeviceStats(ctx, sid, ws.Device)
		if err != nil {
			return meterReading{}, false
		}
		down, up, ok = s.wan.observe(st2.RxBytes, st2.TxBytes)
	}
	if !ok {
		return meterReading{}, false
	}
	link := st.SpeedMbps
	if link <= 0 {
		link = 1000 // the driver reports no speed; a gigabit port is the honest default
	}
	fill := int(math.Round(down * 100 / (float64(link) * 1e6)))
	if fill > 100 {
		fill = 100
	}
	return meterReading{
		Name: "speed", Label: "Speed",
		Value: mbps(down), Unit: "Mbps",
		Fill: fill, Variant: "info",
		Detail: mbps(up) + " Mbps up",
	}, true
}

// meterReadings assembles the current box-health readings: memory from ubus
// system info (sid-gated), storage and CPU from the local kernel.
func (s *Server) meterReadings(ctx context.Context, sid string) []meterReading {
	readings := make([]meterReading, 0, 4)

	if r, ok := s.speedReading(ctx, sid); ok {
		readings = append(readings, r)
	}

	si, siErr := s.backend.SystemInfo(ctx, sid)
	if siErr != nil {
		log.Printf("verso: meters: system info unavailable: %v", siErr)
	} else if si.Memory.Total > 0 {
		used := si.Memory.Total - si.Memory.Available
		readings = append(readings, meterReading{
			Name: "memory", Label: "Memory",
			Value: gb(used), Unit: "GB",
			Fill:   int(used * 100 / si.Memory.Total),
			Detail: gb(si.Memory.Available) + " GB free",
		})
	}

	if st, err := s.stats.Root(); err != nil {
		log.Printf("verso: meters: storage unavailable: %v", err)
	} else if total := st.Used + st.Free; total > 0 {
		readings = append(readings, meterReading{
			Name: "storage", Label: "Storage",
			Value: gb(st.Used), Unit: "GB",
			Fill:   int(st.Used * 100 / total),
			Detail: gb(st.Free) + " GB free",
		})
	}

	if pct, err := s.stats.CPUPercent(); err != nil {
		log.Printf("verso: meters: cpu unavailable: %v", err)
	} else {
		detail := ""
		if siErr == nil {
			detail = "load " + formatLoad(si.Load[0])
		}
		readings = append(readings, meterReading{
			Name: "cpu", Label: "Processor",
			Value: strconv.Itoa(pct), Unit: "%",
			Fill: pct, Detail: detail,
		})
	}

	for i := range readings {
		readings[i].Band = widget.MeterBand(readings[i].Fill, readings[i].Variant)
	}
	return readings
}

// systemMeters assembles the System panel's four bar gauges — load, CPU,
// memory, storage — as live readings with a fixed decorative accent each (not a
// health band). Memory and storage read as a percentage full; load carries its
// raw figure with the bar showing its share of the cores. A failed source is
// omitted (and logged) rather than zeroed.
func (s *Server) systemMeters(ctx context.Context, sid string) []meterReading {
	out := make([]meterReading, 0, 4)
	cores := runtime.NumCPU()

	si, siErr := s.backend.SystemInfo(ctx, sid)
	if siErr != nil {
		log.Printf("verso: system meters: system info unavailable: %v", siErr)
	} else {
		fill := 0
		if cores > 0 {
			fill = int(math.Round(float64(si.Load[0]) / 65536.0 / float64(cores) * 100))
		}
		out = append(out, meterReading{
			Name: "sys-load", Label: "LOAD", Value: formatLoad(si.Load[0]),
			Fill: clampPct(fill), Detail: "1-minute average", Role: "sky",
		})
	}

	if pct, err := s.stats.CPUPercent(); err != nil {
		log.Printf("verso: system meters: cpu unavailable: %v", err)
	} else {
		out = append(out, meterReading{
			Name: "sys-cpu", Label: "CPU", Value: strconv.Itoa(pct), Unit: "%",
			Fill: clampPct(pct), Detail: fmt.Sprintf("%d cores", cores), Role: "violet",
		})
	}

	if siErr == nil && si.Memory.Total > 0 {
		used := si.Memory.Total - si.Memory.Available
		pct := int(used * 100 / si.Memory.Total)
		out = append(out, meterReading{
			Name: "sys-memory", Label: "MEMORY", Value: strconv.Itoa(pct), Unit: "%",
			Fill: clampPct(pct), Detail: "of " + gb(si.Memory.Total) + " GB", Role: "emerald",
		})
	}

	if st, err := s.stats.Root(); err != nil {
		log.Printf("verso: system meters: storage unavailable: %v", err)
	} else if total := st.Used + st.Free; total > 0 {
		pct := int(st.Used * 100 / total)
		out = append(out, meterReading{
			Name: "sys-storage", Label: "STORAGE", Value: strconv.Itoa(pct), Unit: "%",
			Fill: clampPct(pct), Detail: "of " + gb(total) + " GB", Role: "amber",
		})
	}
	return out
}

func clampPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// gb renders a byte count as gigabytes the way a person says them — one
// decimal, trimmed when whole ("23", "1.2", "0.8").
func gb(b int64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(b)/(1<<30)), ".0")
}

// mbps renders a bits-per-second rate likewise — whole Mbps from 10 up, one
// decimal below ("300", "24", "6.2", "0.4").
func mbps(bps float64) string {
	m := bps / 1e6
	if m >= 10 {
		return strconv.Itoa(int(math.Round(m)))
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", m), ".0")
}

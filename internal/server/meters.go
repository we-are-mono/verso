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

	"github.com/we-are-mono/verso/internal/sysstat"
)

// The System panel's meters: the box-health glance — load, CPU, memory,
// storage — as live bar readings. systemMeters builds them for both faces: the
// page render composes meter widgets from it, and the overview stream
// (events.go) pushes the same readings into the rendered bars. A source that
// fails is omitted (and logged), never a 500 — the glance degrades the way the
// status table does.

// statSource is the local-machine seam the meters read — CPU busy share and
// root-filesystem fullness; sysstat.Sampler is the real one, tests fake it.
type statSource interface {
	CPUPercent() (float64, error)
	Root() (sysstat.Storage, error)
}

// meterReading is one meter's worth of truth, shaped for both the widget and
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

// systemMeters assembles the System panel's four bar gauges — load, CPU,
// memory, storage — as live readings with resource-specific health bands.
// Memory and storage read as a percentage full; load carries its
// raw figure with the bar showing its share of the cores. A failed source is
// omitted (and logged) rather than zeroed.
func (s *Server) systemMeters(ctx context.Context, sid string, tr func(string) string) []meterReading {
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
			Name: "sys-load", Label: tr("Load"), Value: formatLoad(si.Load[0]),
			Fill: clampPct(fill), Detail: fmt.Sprintf(tr("1-minute average · %d threads"), cores), Band: systemMeterBand(fill, 60, 85),
		})
	}

	if pct, err := s.stats.CPUPercent(); err != nil {
		log.Printf("verso: system meters: cpu unavailable: %v", err)
	} else {
		out = append(out, meterReading{
			Name: "sys-cpu", Label: tr("CPU"), Value: strings.TrimSuffix(strconv.FormatFloat(pct, 'f', 1, 64), ".0"), Unit: "%",
			Fill: clampPct(int(math.Round(pct))), Detail: fmt.Sprintf(tr("%d threads"), cores), Band: systemMeterBand(int(pct), 70, 90),
		})
	}

	if siErr == nil && si.Memory.Total > 0 {
		used := si.Memory.Total - si.Memory.Available
		pct := int(used * 100 / si.Memory.Total)
		out = append(out, meterReading{
			Name: "sys-memory", Label: tr("Memory"), Value: strconv.Itoa(pct), Unit: "%",
			Fill: clampPct(pct), Detail: fmt.Sprintf(tr("%s of %s GiB"), gb(used), gb(si.Memory.Total)), Band: systemMeterBand(pct, 75, 90),
		})
	}

	if st, err := s.stats.Root(); err != nil {
		log.Printf("verso: system meters: storage unavailable: %v", err)
	} else if total := st.Used + st.Free; total > 0 {
		pct := int(st.Used * 100 / total)
		out = append(out, meterReading{
			Name: "sys-storage", Label: tr("Storage"), Value: strconv.Itoa(pct), Unit: "%",
			Fill: clampPct(pct), Detail: fmt.Sprintf(tr("%s of %s GiB"), gb(st.Used), gb(total)), Band: systemMeterBand(pct, 80, 90),
		})
	}
	return out
}

// Each gauge grades its own resource, using the design's warning/critical limits.
func systemMeterBand(fill, warning, critical int) string {
	if fill >= critical {
		return "danger"
	}
	if fill >= warning {
		return "warning"
	}
	return "success"
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

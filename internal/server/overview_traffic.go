// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/widget"
)

// overviewTrafficPath answers the Internet graph for the window chosen on it.
const overviewTrafficPath = "/overview/traffic"

// trafficWindows lays each past window out as periods of one resolution
// (ADR-019): a day of minutes, a week of quarters, a month of hours, a year
// of days.
var trafficWindows = map[string]struct {
	res history.Resolution
	n   int
}{
	"day":   {history.Minute, 24 * 60},
	"week":  {history.Quarter, 7 * 24 * 4},
	"month": {history.Hour, 30 * 24},
	"year":  {history.Day, 365},
}

// wanMinute is the uplink's live minute for the Internet graph: its device,
// the minute's series in Mbit/s and the newest of each, composed. The
// process-wide sampler normally supplies an already-warm minute; the netifd
// device counters stand in if sampling fails.
func (s *Server) wanMinute(r *http.Request) (device string, down, up []float64, downNow, upNow string) {
	wan, err := s.wanStatus(r)
	primary, ok := wan.Primary()
	if err == nil && ok {
		device = primary.Device
		if snapshot, fresh := s.telemetrySnapshot(r.Context()); fresh {
			if iface, found := snapshot.Interface(device); found {
				down, up = telemetryInterfaceRates(iface.History)
			}
		}
	}
	var d, u float64
	if len(down) != 0 {
		d, u = down[len(down)-1], up[len(up)-1]
	} else {
		down, up = s.wanHist.Series()
		d, u = s.wanHist.Latest()
	}
	return device, down, up, fmt.Sprintf("%.1f", d), fmt.Sprintf("%.1f", u)
}

// handleOverviewTraffic answers the Internet graph for ?range=: the live
// minute, or a past window drawn from the uplink's history.
func (s *Server) handleOverviewTraffic(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		http.NotFound(w, r)
		return
	}
	window := r.URL.Query().Get("range")
	var traffic *widget.Traffic
	if window == widget.TrafficRanges[0].Value {
		device, down, up, downNow, upNow := s.wanMinute(r)
		traffic = widget.InternetTraffic(device, downNow, upNow, down, up, overviewTrafficPath, window)
	} else {
		layout, offered := trafficWindows[window]
		if !offered {
			http.Error(w, "no such window", http.StatusBadRequest)
			return
		}
		var device string
		if wan, err := s.wanStatus(r); err == nil {
			if primary, ok := wan.Primary(); ok {
				device = primary.Device
			}
		}
		step := time.Duration(layout.res) * time.Second
		start := time.Now().Truncate(step).Add(-time.Duration(layout.n-1) * step)
		down, err := s.wanWindow(r, "iface."+device+".rx", layout.res, start, layout.n)
		if err != nil {
			http.Error(w, "history unavailable", http.StatusInternalServerError)
			return
		}
		up, err := s.wanWindow(r, "iface."+device+".tx", layout.res, start, layout.n)
		if err != nil {
			http.Error(w, "history unavailable", http.StatusInternalServerError)
			return
		}
		traffic = widget.InternetTraffic(device, mean(down), mean(up), down, up, overviewTrafficPath, window)
		end := start.Add(time.Duration(layout.n) * step)
		events, err := s.historyStore.Events(r.Context(), time.Unix(0, 0), end)
		if err != nil {
			http.Error(w, "history unavailable", http.StatusInternalServerError)
			return
		}
		_, t := s.localize(r)
		traffic.Marks = eventMarks(events, start, end, time.Now(), translatorOrIdentity(t))
	}
	var body strings.Builder
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, traffic, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body.String()))
}

// wanWindow is one series' averages over n periods from start, in Mbit/s,
// NaN where nothing was recorded.
func (s *Server) wanWindow(r *http.Request, series string, res history.Resolution, start time.Time, n int) ([]float64, error) {
	end := start.Add(time.Duration(n) * time.Duration(res) * time.Second)
	points, err := s.historyStore.Points(r.Context(), series, res, start, end)
	if err != nil {
		return nil, err
	}
	return history.Slots(points, func(p history.Point) float64 { return rateMbps(uint64(p.Average)) }, start, res, n), nil
}

// mean is the average of what was recorded, composed as the graph's figures
// are; a window with nothing recorded reads as zero.
func mean(values []float64) string {
	sum, n := 0.0, 0
	for _, v := range values {
		if !math.IsNaN(v) {
			sum += v
			n++
		}
	}
	if n == 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", sum/float64(n))
}

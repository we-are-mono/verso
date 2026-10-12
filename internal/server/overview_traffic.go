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
// the minute's series in Mbit/s and the newest of each. The process-wide
// sampler normally supplies an already-warm minute; the netifd device counters
// stand in if sampling fails.
func (s *Server) wanMinute(r *http.Request) (device string, down, up []float64, downNow, upNow float64) {
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
	if len(down) != 0 {
		return device, down, up, down[len(down)-1], up[len(up)-1]
	}
	down, up = s.wanHist.Series()
	downNow, upNow = s.wanHist.Latest()
	return device, down, up, downNow, upNow
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
		var err error
		if traffic, err = s.pastTraffic(r, window, layout.res, layout.n); err != nil {
			http.Error(w, "history unavailable", http.StatusInternalServerError)
			return
		}
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

// pastTraffic is the Internet graph over n periods of res ending now: the
// uplink's traffic, its events and its stretches of packet loss as marks, and
// the gateway's average latency beside the traffic's.
func (s *Server) pastTraffic(r *http.Request, window string, res history.Resolution, n int) (*widget.Traffic, error) {
	var device string
	if wan, err := s.wanStatus(r); err == nil {
		if primary, ok := wan.Primary(); ok {
			device = primary.Device
		}
	}
	now := time.Now()
	step := time.Duration(res) * time.Second
	start := now.Truncate(step).Add(-time.Duration(n-1) * step)
	end := start.Add(time.Duration(n) * step)
	read := func(series string, value func(float64) float64) ([]float64, error) {
		points, err := s.historyStore.Points(r.Context(), series, res, start, end)
		return history.Slots(points, func(p history.Point) float64 { return value(p.Average) }, start, res, n), err
	}
	asIs := func(v float64) float64 { return v }
	mbps := func(v float64) float64 { return rateMbps(uint64(v)) }
	down, err := read("iface."+device+".rx", mbps)
	if err != nil {
		return nil, err
	}
	up, err := read("iface."+device+".tx", mbps)
	if err != nil {
		return nil, err
	}
	loss, err := read("wan.gateway.loss", asIs)
	if err != nil {
		return nil, err
	}
	latency, err := read("wan.gateway.latency", asIs)
	if err != nil {
		return nil, err
	}
	events, err := s.historyStore.Events(r.Context(), time.Unix(0, 0), end)
	if err != nil {
		return nil, err
	}
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	traffic := widget.InternetTraffic(device, mean(down), mean(up), down, up, overviewTrafficPath, window)
	traffic.Marks = eventMarks(events, start, end, now, tr)
	traffic.Marks = append(traffic.Marks, lossMarks(loss, start, step, traffic.Marks, now, tr)...)
	if ms, ok := average(latency); ok {
		traffic.Latency = fmt.Sprintf("%.0f", ms)
		if ms < 10 {
			traffic.Latency = fmt.Sprintf("%.1f", ms)
		}
	}
	return traffic, nil
}

// mean is the average of what was recorded; a window with nothing recorded
// reads as zero.
func mean(values []float64) float64 {
	v, _ := average(values)
	return v
}

// average is the mean of what was recorded; ok is false when nothing was.
func average(values []float64) (float64, bool) {
	sum, n := 0.0, 0
	for _, v := range values {
		if !math.IsNaN(v) {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

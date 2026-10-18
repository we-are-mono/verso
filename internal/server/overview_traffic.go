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
		_, down, up, downNow, upNow := s.wanMinute(r)
		traffic = widget.InternetTraffic(downNow, upNow, down, up, window)
	} else {
		layout, offered := historyWindowOf(window)
		if !offered {
			http.Error(w, "no such window", http.StatusBadRequest)
			return
		}
		// The window is history.db's, the shell's own store: the uplink's
		// readings reach a session only under the interface dump's grant.
		g := s.grantsFor(s.sessionSID(r))
		if !g.may(r.Context(), interfaceGrant) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		var err error
		if traffic, err = s.pastTraffic(r, g, layout); err != nil {
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

// pastTraffic is the Internet graph over a past window ending now: the
// uplink's traffic, its events and its stretches of packet loss as marks, and
// the gateway's average latency beside the traffic's. Each event is marked
// only under its kind's grant (eventGrants), as the events page lists it.
func (s *Server) pastTraffic(r *http.Request, g *sessionGrants, w historyWindow) (*widget.Traffic, error) {
	var device string
	if wan, err := s.wanStatus(r); err == nil {
		if primary, ok := wan.Primary(); ok {
			device = primary.Device
		}
	}
	now := time.Now()
	res, n, step := w.Res, w.N, w.step()
	start, end := w.bounds(now)
	// read is a series' averages over the window, slot by slot, and the
	// highest peak among its periods.
	read := func(series string, value func(float64) float64) ([]float64, float64, error) {
		points, err := s.historyStore.Points(r.Context(), series, res, start, end)
		peak := 0.0
		for _, p := range points {
			peak = max(peak, value(p.Peak))
		}
		return history.Slots(points, func(p history.Point) float64 { return value(p.Average) }, start, res, n), peak, err
	}
	asIs := func(v float64) float64 { return v }
	mbps := func(v float64) float64 { return rateMbps(uint64(v)) }
	down, downPeak, err := read("iface."+device+".rx", mbps)
	if err != nil {
		return nil, err
	}
	up, upPeak, err := read("iface."+device+".tx", mbps)
	if err != nil {
		return nil, err
	}
	loss, _, err := read("wan.gateway.loss", asIs)
	if err != nil {
		return nil, err
	}
	latency, _, err := read("wan.gateway.latency", asIs)
	if err != nil {
		return nil, err
	}
	events, err := s.historyStore.Events(r.Context(), time.Unix(0, 0), end)
	if err != nil {
		return nil, err
	}
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	traffic := widget.InternetTraffic(mean(down), mean(up), down, up, w.Value)
	traffic.DownPeak, traffic.UpPeak = downPeak, upPeak
	traffic.Marks = eventMarks(grantedEvents(r.Context(), g, events), start, end, now, tr)
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

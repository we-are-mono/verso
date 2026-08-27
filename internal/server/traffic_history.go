// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"math"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/sysstat"
)

// trafficHistory turns conntrack's running totals into per-address rate
// history: the overview stream observes the totals once a second (it already
// holds a session and a clock), and each observation becomes one point per
// address. The device drawers then draw the last minute as a sparkline.
// In-memory only — history exists while the shell runs, like the flows it
// measures.

// trafficPoints is how many samples the ring keeps — one a second, a minute
// of story.
const trafficPoints = 60

// trafficPoint is one observation: per-address down/up rates in bits/sec.
type trafficPoint struct {
	down, up map[string]float64
}

type trafficHistory struct {
	now func() time.Time

	mu     sync.Mutex
	last   map[string]sysstat.DeviceTraffic
	lastAt time.Time
	ring   [trafficPoints]trafficPoint
	idx    int // next slot to write
	filled int
}

func newTrafficHistory(now func() time.Time) *trafficHistory {
	return &trafficHistory{now: now}
}

// Observe folds in a fresh per-address totals snapshot. Concurrent streams
// all observe; anything closer than ~a second to the previous observation is
// dropped, so overlapping viewers don't double the tempo.
func (h *trafficHistory) Observe(byAddr map[string]sysstat.DeviceTraffic) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	dt := now.Sub(h.lastAt).Seconds()
	if h.last != nil && dt < 0.9 {
		return
	}
	if h.last != nil && dt > 0 {
		point := trafficPoint{down: map[string]float64{}, up: map[string]float64{}}
		for addr, cur := range byAddr {
			prev, ok := h.last[addr]
			if !ok {
				continue
			}
			if drx := cur.RxBytes - prev.RxBytes; drx > 0 {
				point.down[addr] = float64(drx) * 8 / dt
			}
			if dtx := cur.TxBytes - prev.TxBytes; dtx > 0 {
				point.up[addr] = float64(dtx) * 8 / dt
			}
		}
		h.ring[h.idx] = point
		h.idx = (h.idx + 1) % trafficPoints
		if h.filled < trafficPoints {
			h.filled++
		}
	}
	h.last = byAddr
	h.lastAt = now
}

// Series sums the history across a device's addresses — oldest to newest, in
// Mbps, always the full minute: a part-filled ring left-pads with quiet, so
// the time axis never compresses as points arrive. Empty until anything has
// been observed.
func (h *trafficHistory) Series(addrs []string) (down, up []float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.filled == 0 {
		return nil, nil
	}
	down = make([]float64, trafficPoints)
	up = make([]float64, trafficPoints)
	pad := trafficPoints - h.filled
	start := (h.idx - h.filled + trafficPoints) % trafficPoints
	for i := 0; i < h.filled; i++ {
		p := h.ring[(start+i)%trafficPoints]
		for _, addr := range addrs {
			down[pad+i] += p.down[addr] / 1e6
			up[pad+i] += p.up[addr] / 1e6
		}
		// Two decimals is plenty for a plot and keeps the streamed JSON slim.
		down[pad+i] = math.Round(down[pad+i]*100) / 100
		up[pad+i] = math.Round(up[pad+i]*100) / 100
	}
	return down, up
}

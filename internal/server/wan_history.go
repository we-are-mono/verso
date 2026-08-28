// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"math"
	"sync"
	"time"
)

// wanHistory turns the WAN device's byte counters into a minute of throughput:
// the overview stream observes the counters once a second (it holds a session
// and a clock), and each observation becomes one down/up point in Mbps. The
// overview graph draws the ring on load, already populated. In-memory only —
// history exists while the shell runs, and it fills while a viewer streams,
// since every counter read is gated on that viewer's session (ADR-007), so there
// is no always-on sampler. A cold first load fills over the minute.
const wanPoints = 60

type wanHistory struct {
	now func() time.Time

	mu             sync.Mutex
	lastRx, lastTx int64
	lastAt         time.Time
	have           bool
	down, up       [wanPoints]float64
	idx            int // next slot to write
	filled         int
	curDown, curUp float64
}

func newWanHistory(now func() time.Time) *wanHistory {
	return &wanHistory{now: now}
}

// Observe folds in a fresh counter reading. Concurrent streams all observe;
// anything closer than ~a second to the previous observation is dropped so
// overlapping viewers don't double the tempo. A counter that goes backwards (an
// interface reset) reads as a quiet sample rather than a negative spike.
func (h *wanHistory) Observe(rx, tx int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	dt := now.Sub(h.lastAt).Seconds()
	if h.have && dt < 0.9 {
		return
	}
	if h.have && dt > 0 {
		var d, u float64
		if drx := rx - h.lastRx; drx > 0 {
			d = math.Round(float64(drx)*8/dt/1e6*100) / 100
		}
		if dtx := tx - h.lastTx; dtx > 0 {
			u = math.Round(float64(dtx)*8/dt/1e6*100) / 100
		}
		h.down[h.idx] = d
		h.up[h.idx] = u
		h.idx = (h.idx + 1) % wanPoints
		if h.filled < wanPoints {
			h.filled++
		}
		h.curDown, h.curUp = d, u
	}
	h.lastRx, h.lastTx, h.lastAt, h.have = rx, tx, now, true
}

// Series returns the down/up history oldest to newest, in Mbps, always the full
// minute: a part-filled ring left-pads with quiet so the time axis never
// compresses as points arrive. Nil until anything has been observed.
func (h *wanHistory) Series() (down, up []float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.filled == 0 {
		return nil, nil
	}
	down = make([]float64, wanPoints)
	up = make([]float64, wanPoints)
	pad := wanPoints - h.filled
	start := (h.idx - h.filled + wanPoints) % wanPoints
	for i := 0; i < h.filled; i++ {
		p := (start + i) % wanPoints
		down[pad+i] = h.down[p]
		up[pad+i] = h.up[p]
	}
	return down, up
}

// Latest is the newest down/up sample in Mbps (0 before anything is observed).
func (h *wanHistory) Latest() (down, up float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.curDown, h.curUp
}

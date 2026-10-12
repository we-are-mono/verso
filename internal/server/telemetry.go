// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/units"
)

const (
	telemetryReadCache = 500 * time.Millisecond
	telemetryMaxAge    = 3 * time.Second
	telemetrySeriesLen = 60
)

type telemetrySource interface {
	Snapshot(context.Context) (telemetry.Snapshot, error)
}

type interfaceReading struct {
	Name      string `json:"name"`
	Operstate string `json:"operstate"`
	State     string `json:"state"`
	Variant   string `json:"variant"`
	RxRate    string `json:"rx_rate"`
	TxRate    string `json:"tx_rate"`
}

// telemetrySnapshot shares one immutable in-process snapshot across the page
// and every SSE connection in a tick. A stale sample is treated as unavailable.
func (s *Server) telemetrySnapshot(ctx context.Context) (telemetry.Snapshot, bool) {
	s.telemetryMu.Lock()
	defer s.telemetryMu.Unlock()
	now := time.Now()
	if s.telemetryAt.IsZero() || now.Sub(s.telemetryAt) >= telemetryReadCache {
		s.telemetrySnap, s.telemetryErr = s.telemetry.Snapshot(ctx)
		s.telemetryAt = now
	}
	return s.telemetrySnap, s.telemetryErr == nil && s.telemetrySnap.Fresh(now, telemetryMaxAge)
}

func telemetryInterfaceRates(history []telemetry.Point) (down, up []float64) {
	if len(history) == 0 {
		return nil, nil
	}
	if len(history) > telemetrySeriesLen {
		history = history[len(history)-telemetrySeriesLen:]
	}
	down = make([]float64, telemetrySeriesLen)
	up = make([]float64, telemetrySeriesLen)
	pad := telemetrySeriesLen - len(history)
	for index, point := range history {
		down[pad+index] = rateMbps(point.RxBPS)
		up[pad+index] = rateMbps(point.TxBPS)
	}
	return down, up
}

func rateMbps(bitsPerSecond uint64) float64 {
	return math.Round(float64(bitsPerSecond)/1e6*100) / 100
}

// telemetryInterfaceReadings is each interface's state and rates now, the
// rates written in the reader's language.
func telemetryInterfaceReadings(snapshot telemetry.Snapshot, nums units.Numbers) []interfaceReading {
	out := make([]interfaceReading, 0, len(snapshot.Interfaces))
	for _, iface := range snapshot.Interfaces {
		state := normalOperstate(iface.Operstate)
		variant := "neutral"
		if state == "up" {
			variant = "success"
		}
		var rxBPS, txBPS uint64
		if len(iface.History) != 0 {
			point := iface.History[len(iface.History)-1]
			rxBPS, txBPS = point.RxBPS, point.TxBPS
		}
		rx, tx := nums.Rate(float64(rxBPS)).String(), nums.Rate(float64(txBPS)).String()
		out = append(out, interfaceReading{
			Name: iface.Name, Operstate: state, State: strings.ToUpper(state[:1]) + state[1:],
			Variant: variant, RxRate: rx, TxRate: tx,
		})
	}
	return out
}

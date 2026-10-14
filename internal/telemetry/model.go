// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package telemetry samples and retains kernel network-interface counters.
package telemetry

import "time"

type Point struct {
	TimestampMS uint64
	RxBytes     uint64
	TxBytes     uint64
	RxPackets   uint64
	TxPackets   uint64
	RxBPS       uint64
	TxBPS       uint64
	RxPPS       uint64
	TxPPS       uint64
}

type Interface struct {
	Name      string
	Operstate string
	Kind      string
	Physical  bool
	// Conduit marks a DSA conduit: the physical NIC that links a switch chip
	// to the SoC, whose ports are the switch's own netdevs, not it.
	Conduit bool
	// SpeedMbps is a physical port's negotiated link speed, -1 when it has no
	// link, reports none, or is not a port.
	SpeedMbps int
	Parent    string
	Members   []string
	History   []Point
}

type Snapshot struct {
	TimestampMS uint64
	Interfaces  []Interface
}

func (s Snapshot) Fresh(now time.Time, maxAge time.Duration) bool {
	if s.TimestampMS == 0 {
		return false
	}
	age := now.Sub(time.UnixMilli(int64(s.TimestampMS)))
	return age >= -time.Second && age <= maxAge
}

func (s Snapshot) Interface(name string) (Interface, bool) {
	for _, candidate := range s.Interfaces {
		if candidate.Name == name {
			return candidate, true
		}
	}
	return Interface{}, false
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package ports knows a router's physical network ports as its case shows
// them: which interfaces are ports, the order they stand in, what kind of
// connector each is, and how its LEDs and label read.
package ports

import (
	"hash/fnv"
	"strconv"
)

// Activity is the level a port's traffic LED shows for the packets it moves
// each second, both directions together: 0 dark, 1 an occasional blip, 2 a
// steady blink, 3 a fast flicker, 4 a near-solid flicker.
func Activity(pps uint64) int {
	switch {
	case pps == 0:
		return 0
	case pps < 10:
		return 1
	case pps < 1000:
		return 2
	case pps < 100000:
		return 3
	default:
		return 4
	}
}

// Phase is how far into its blink a port's traffic LED starts, as one of
// eight fractions of a period, so busy ports do not blink in step. It comes
// from the port's name, so a port keeps it from one page to the next.
func Phase(name string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return float64(h.Sum32()%8) / 8
}

// SpeedLabel is a port's negotiated speed as its label reads it (100M, 1G,
// 2.5G), a dash when it has no link.
func SpeedLabel(mbps int) string {
	switch {
	case mbps <= 0:
		return "—"
	case mbps < 1000:
		return strconv.Itoa(mbps) + "M"
	default:
		return strconv.FormatFloat(float64(mbps)/1000, 'f', -1, 64) + "G"
	}
}

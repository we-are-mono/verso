// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package units writes the shell's measured quantities, rates and amounts of
// data, one way everywhere and in the reader's language. A figure keeps one
// decimal under ten and is whole from ten, its ".0" dropped. A rate is in Mb/s
// up to 999 and in Gb/s from there. A rate above zero reads at least 0.1 Mb/s,
// so a trickle is never mistaken for silence.
//
// The page writes live figures with the same rule (verso-units.js); both are
// held to the cases in testdata/rates.json.
package units

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Numbers writes numbers the way one language does.
type Numbers struct {
	decimal string
}

// For is how the language tr translates into writes numbers. Its decimal
// separator is the catalog's translation of "." (ADR-012), so a language
// brings it in its catalog with no rebuild of the shell. A nil tr is English.
func For(tr func(string) string) Numbers {
	n := Numbers{decimal: "."}
	if tr != nil {
		if d := tr("."); d != "" {
			n.decimal = d
		}
	}
	return n
}

// Rate is a rate written for reading: its figure, and what it is counted in,
// apart so a figure can stand large beside its unit.
type Rate struct {
	Value, Unit string
}

// String is the rate as one phrase, "4.3 Mb/s".
func (r Rate) String() string { return r.Value + " " + r.Unit }

// AxisFractions are where a chart's value labels stand, top first.
var AxisFractions = []float64{1, 0.75, 0.5, 0.25}

// Figure writes v at its scale: one decimal under ten, whole from ten.
func (n Numbers) Figure(v float64) string {
	tenths := math.Round(v*10) / 10
	if tenths >= 10 {
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
	}
	s := strings.TrimSuffix(strconv.FormatFloat(tenths, 'f', 1, 64), ".0")
	return strings.Replace(s, ".", n.decimal, 1)
}

// Rate writes a rate of bps bits per second.
func (n Numbers) Rate(bps float64) Rate {
	if !(bps > 0) {
		return Rate{"0", "Mb/s"}
	}
	unit, per := rateScale(max(bps, 100_000))
	return Rate{n.Figure(max(bps, 100_000) / per), unit}
}

// RateAxis writes a chart's value labels for a plot topped at top bits per
// second, one per fraction: the first is that rate in full, the rest are
// figures in its unit.
func (n Numbers) RateAxis(top float64, fractions []float64) []string {
	unit, per := rateScale(top)
	labels := make([]string, len(fractions))
	for i, f := range fractions {
		labels[i] = n.Figure(top * f / per)
	}
	if len(labels) != 0 {
		labels[0] += " " + unit
	}
	return labels
}

// rateScale is the unit a rate of bps is written in, and the bits per second
// in one of it. A rate that rounds to 1000 Mb/s is already 1 Gb/s.
func rateScale(bps float64) (unit string, per float64) {
	if mb := bps / 1e6; math.Round(mb) >= 1000 {
		return "Gb/s", 1e9
	}
	return "Mb/s", 1e6
}

// Bytes writes a period's total at its scale, in binary units: whole MiB under
// a GiB, a decimal under ten GiB, whole GiB under a TiB. Nothing is the empty
// string; anything is at least 1 MiB.
func (n Numbers) Bytes(b uint64) string {
	const mib, gib, tib = 1 << 20, 1 << 30, 1 << 40
	switch {
	case b == 0:
		return ""
	case b < gib:
		return fmt.Sprintf("%d MiB", max(1, int(math.Ceil(float64(b)/mib))))
	case b < tib:
		return n.Figure(float64(b)/gib) + " GiB"
	default:
		return n.Figure(float64(b)/tib) + " TiB"
	}
}

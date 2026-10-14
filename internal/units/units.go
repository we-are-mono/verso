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
	tr      func(string) string
}

// For is how the language tr translates into writes numbers. Its decimal
// separator is the catalog's translation of "." (ADR-012), so a language
// brings it in its catalog with no rebuild of the shell. A nil tr is English.
func For(tr func(string) string) Numbers {
	n := Numbers{decimal: ".", tr: func(s string) string { return s }}
	if tr != nil {
		n.tr = tr
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

// AxisFractions are where a chart's value labels stand, top first: the top
// and its three quarter lines, so a round top of four steps labels each step.
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

// RateScale is a chart's scale for a plot whose tallest point is peak bits per
// second: a round top, in bits per second, and its labels at AxisFractions,
// the first that rate in full and the rest figures in its unit. The top is
// four round steps (roundStep), at least a tenth over the peak so the curve
// never touches it, and at least 0.4 Mb/s. Its labels read as counted
// (60 · 45 · 30 · 15), the curve fills at least three fifths of the plot, and
// the scale moves only when the traffic crosses a step, not with every
// second's peak.
func (n Numbers) RateScale(peak float64) (top float64, labels []string) {
	if !(peak > 0) || math.IsInf(peak, 1) {
		peak = 0 // nothing recorded, or nothing a chart could draw
	}
	need := max(peak*1.1, 400_000)
	unit, per := "Mb/s", 1e6
	step := roundStep(need / per / 4)
	if 4*step >= 1000 {
		unit, per = "Gb/s", 1e9
		step = roundStep(need / per / 4)
	}
	labels = make([]string, len(AxisFractions))
	for i, f := range AxisFractions {
		labels[i] = n.Figure(4 * step * f)
	}
	labels[0] += " " + unit
	return 4 * step * per, labels
}

// roundSteps are the steps a scale counts in, times a power of ten: close
// enough together that the next one up is never more than half again.
var roundSteps = []float64{1, 1.5, 2, 2.5, 3, 4, 5, 6, 8}

// roundStep is the smallest round step at least v that is whole tenths: a
// figure keeps one decimal, so a step of 0.25 would write its 0.75 as 0.8.
func roundStep(v float64) float64 {
	for p := math.Pow10(int(math.Floor(math.Log10(v)))); ; p *= 10 {
		for _, m := range roundSteps {
			s := m * p
			if tenths := s * 10; math.Abs(tenths-math.Round(tenths)) > 1e-9 {
				continue
			}
			if s >= v*(1-1e-9) {
				return s
			}
		}
	}
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

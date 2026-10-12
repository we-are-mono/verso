// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package units

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// fixtures are the cases testdata/rates.json states once for the shell and the
// page (scripts/units.test.cjs).
type fixtures struct {
	Rates []struct {
		BPS   float64 `json:"bps"`
		Value string  `json:"value"`
		Unit  string  `json:"unit"`
	} `json:"rates"`
	Scales []struct {
		PeakBPS float64  `json:"peak_bps"`
		TopBPS  float64  `json:"top_bps"`
		Labels  []string `json:"labels"`
	} `json:"scales"`
}

func readFixtures(t *testing.T) fixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/rates.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// comma is a language that writes "4,3".
func comma(s string) string {
	if s == "." {
		return ","
	}
	return s
}

func TestRatesReadAtTheirScale(t *testing.T) {
	for _, tc := range readFixtures(t).Rates {
		if got := For(nil).Rate(tc.BPS); got != (Rate{tc.Value, tc.Unit}) {
			t.Errorf("Rate(%v) = %+v, want %s %s", tc.BPS, got, tc.Value, tc.Unit)
		}
		want := strings.ReplaceAll(tc.Value, ".", ",") + " " + tc.Unit
		if got := For(comma).Rate(tc.BPS).String(); got != want {
			t.Errorf("Rate(%v) with a decimal comma = %q, want %q", tc.BPS, got, want)
		}
	}
}

func TestAChartScaleIsRound(t *testing.T) {
	for _, tc := range readFixtures(t).Scales {
		top, got := For(nil).RateScale(tc.PeakBPS)
		if math.Abs(top-tc.TopBPS) > 1 || strings.Join(got, "|") != strings.Join(tc.Labels, "|") {
			t.Errorf("RateScale(%v) = %v %q, want %v %q", tc.PeakBPS, top, got, tc.TopBPS, tc.Labels)
		}
		_, got = For(comma).RateScale(tc.PeakBPS)
		if want := strings.ReplaceAll(strings.Join(tc.Labels, "|"), ".", ","); strings.Join(got, "|") != want {
			t.Errorf("RateScale(%v) with a decimal comma = %q, want %q", tc.PeakBPS, got, want)
		}
	}
}

func TestAScaleOverNothingIsTheSmallest(t *testing.T) {
	for _, peak := range []float64{math.NaN(), math.Inf(1), -3} {
		if top, _ := For(nil).RateScale(peak); top != 400_000 {
			t.Errorf("RateScale(%v) = %v, want the smallest scale", peak, top)
		}
	}
}

func TestFiguresDropTheirDecimalFromTen(t *testing.T) {
	for v, want := range map[float64]string{0: "0", 0.25: "0.3", 3: "3", 9.94: "9.9", 9.96: "10", 34.5: "35", 412.7: "413"} {
		if got := For(nil).Figure(v); got != want {
			t.Errorf("Figure(%v) = %q, want %q", v, got, want)
		}
	}
	if got := For(comma).Figure(2.5); got != "2,5" {
		t.Errorf("Figure(2.5) with a decimal comma = %q, want 2,5", got)
	}
}

func TestBytesReadAtTheirScale(t *testing.T) {
	for b, want := range map[uint64]string{0: "", 512: "1 MiB", 300 << 20: "300 MiB", 3 << 29: "1.5 GiB", 212 << 30: "212 GiB", 3 << 39: "1.5 TiB"} {
		if got := For(nil).Bytes(b); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", b, got, want)
		}
	}
	if got := For(comma).Bytes(3 << 29); got != "1,5 GiB" {
		t.Errorf("Bytes(1.5 GiB) with a decimal comma = %q, want 1,5 GiB", got)
	}
}

func TestALanguageWithoutASeparatorKeepsThePoint(t *testing.T) {
	blank := func(string) string { return "" }
	if got := For(blank).Figure(2.5); got != "2.5" {
		t.Errorf("Figure(2.5) with no separator translated = %q, want 2.5", got)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderChart: the plot stretches edge to edge, its peak sits under 15%
// headroom, the value labels ride the quarter lines with the unit on the
// topmost, and the window is captioned at its two ends.
func TestRenderChart(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{
		Unit: "Mbit/s", AxisStart: "60 s ago", AxisEnd: "now", Label: "Traffic",
		Series: []ChartSeries{
			{Values: []float64{0, 100}, Role: "emerald", Fill: true},
			{Values: []float64{0, 50}, Role: "violet"},
		},
	})
	for _, want := range []string{
		`preserveAspectRatio="none"`, `aria-label="Traffic"`, `data-chart-padding="0"`,
		"verso-chart--emerald", "verso-chart--violet",
		"620.0 31.3", // 100 under a top of 115 lands at 240 - 100/115·240
		">115 Mbit/s<", ">86<", ">57<", ">29<", `style="top:0.0%"`, `style="top:75.0%"`,
		">60 s ago<", ">now<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chart missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "verso-chart-area"); n != 1 {
		t.Errorf("only the filled series draws an area, got %d", n)
	}
}

// TestChartGradientIdsUnique: two charts rendered by one renderer get distinct gradient
// ids, so their <defs> never collide on a shared page.
func TestChartGradientIdsUnique(t *testing.T) {
	r := newRenderer(t)
	spec := &Chart{Series: []ChartSeries{{Values: []float64{1, 2, 3}, Fill: true}}}
	first := render(t, r, spec)
	second := render(t, r, spec)
	if !strings.Contains(first, `id="vc1-0"`) {
		t.Errorf("first chart should own gradient vc1-0:\n%s", first)
	}
	if !strings.Contains(second, `id="vc2-0"`) {
		t.Errorf("second chart should own gradient vc2-0:\n%s", second)
	}
}

// TestChartCurveDoesNotOvershootEndpoints: a flat zero followed by a rise used
// to pull the preceding spline segment below the graph's zero baseline.
func TestChartCurveDoesNotOvershootEndpoints(t *testing.T) {
	got := chartCurve([][2]float64{{0, 100}, {1, 100}, {2, 0}})
	if strings.Contains(got, "116.7") {
		t.Fatalf("curve crossed below the zero baseline: %s", got)
	}
	if !strings.Contains(got, "C0.2 100.0 0.7 100.0 1.0 100.0") {
		t.Fatalf("first segment's controls were not constrained to its endpoints: %s", got)
	}
}

// TestChartSkipsShortSeries: a series with fewer than two points can't be a line — it is
// skipped, not a panic or a broken path.
func TestChartSkipsShortSeries(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{Series: []ChartSeries{{Values: []float64{42}}}})
	if !strings.Contains(got, "<svg") {
		t.Errorf("a short series should still yield an svg frame:\n%s", got)
	}
	if strings.Contains(got, "verso-chart-line") {
		t.Errorf("a one-point series must draw no line:\n%s", got)
	}
}

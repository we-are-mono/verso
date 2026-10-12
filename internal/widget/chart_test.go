// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"math"
	"strings"
	"testing"
)

// TestRenderChart: the plot stretches edge to edge, its peak sits under 15%
// headroom, the value labels ride the quarter lines with the unit on the
// topmost, and the window is captioned at its two ends.
func TestRenderChart(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{
		AxisStart: "60 s ago", AxisEnd: "now", Label: "Traffic",
		Series: []ChartSeries{
			{Values: []float64{0, 100}, Role: "emerald", Fill: true},
			{Values: []float64{0, 50}, Role: "violet"},
		},
	})
	for _, want := range []string{
		`preserveAspectRatio="none"`, `aria-label="Traffic"`, `data-chart-padding="0"`,
		"verso-chart--emerald", "verso-chart--violet",
		"620.0 31.3", // 100 under a top of 115 lands at 240 - 100/115·240
		">115 Mb/s<", ">86<", ">57<", ">29<", `style="top:0.0%"`, `style="top:75.0%"`,
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

// TestChartBreaksTheLineAtAGap: a NaN is a period nothing was recorded, so the
// line stops there and starts again after it, each run in its place on the
// axis; the scale ignores the gap, and a lone point between gaps is no line.
func TestChartBreaksTheLineAtAGap(t *testing.T) {
	nan := math.NaN()
	got := render(t, newRenderer(t), &Chart{Series: []ChartSeries{
		{Values: []float64{1, 2, nan, 5, nan, 4, 4}, Fill: true},
	}})
	if strings.Contains(got, "NaN") {
		t.Fatalf("a gap was drawn as a number:\n%s", got)
	}
	line := between(got, `class="verso-chart-line`, `/>`)
	if n := strings.Count(line, "M"); n != 2 {
		t.Fatalf("line has %d runs, want 2 (the lone 5 is no line): %s", n, line)
	}
	// Seven slots over 620: the second run starts at slot 5, x = 516.7.
	if !strings.Contains(line, "M516.7 ") {
		t.Fatalf("the second run is not at its own place on the axis: %s", line)
	}
	if area := between(got, `class="verso-chart-area"`, `/>`); strings.Count(area, "Z") != 2 {
		t.Fatalf("area does not close each run on its own: %s", area)
	}
}

// TestChartMarksWhatHappened: an outage is a band from where the internet went
// down to where it came back, a boot a rule at its moment, each titled for
// the pointer, and both behind the curves so the data stays on top.
func TestChartMarksWhatHappened(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{
		Series: []ChartSeries{{Values: []float64{1, 2, 3}, Role: "emerald"}},
		Marks: []ChartMark{
			{Kind: "outage", From: 0.25, To: 0.5, Label: "Internet down 03:12–03:20"},
			{Kind: "boot", From: 0.75, Label: "Router started at 04:00"},
		},
	})
	for _, want := range []string{
		`<rect class="verso-chart-mark--outage" x="155.0" y="0" width="155.0" height="240"><title>Internet down 03:12–03:20</title></rect>`,
		`<line class="verso-chart-mark--boot" x1="465.0" x2="465.0" y1="0" y2="240" vector-effect="non-scaling-stroke"><title>Router started at 04:00</title></line>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chart missing %s\n%s", want, got)
		}
	}
	if strings.Index(got, "verso-chart-mark--outage") > strings.Index(got, "verso-chart-series") {
		t.Error("marks are drawn over the curves")
	}

	// Packet loss is a band too, in its own colour.
	lossy := render(t, newRenderer(t), &Chart{Marks: []ChartMark{{Kind: "loss", From: 0, To: 0.5, Label: "Packet loss up to 23 %"}}})
	if !strings.Contains(lossy, `<rect class="verso-chart-mark--loss" x="0.0" y="0" width="310.0"`) {
		t.Errorf("a loss band is not drawn:\n%s", lossy)
	}

	// A 25-second outage on a day's chart is a sliver; it keeps a width the
	// eye can find.
	brief := render(t, newRenderer(t), &Chart{Marks: []ChartMark{{Kind: "outage", From: 0.5, To: 0.5003}}})
	if !strings.Contains(brief, `x="310.0" y="0" width="1.5"`) {
		t.Errorf("a brief outage is not kept visible:\n%s", brief)
	}
}

// between is the text after the first start and before the next end.
func between(s, start, end string) string {
	_, rest, _ := strings.Cut(s, start)
	before, _, _ := strings.Cut(rest, end)
	return before
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

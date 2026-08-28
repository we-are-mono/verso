// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderChartFull: the hero graph — two series, a filled area under the first and a
// bare line for the second, with faint gridlines and an endpoint dot.
func TestRenderChartFull(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size:  "full",
		Label: "Download and upload, last minute",
		Series: []ChartSeries{
			{Label: "Download", Values: []float64{280, 300, 260, 320, 300}, Role: "sky", Fill: true},
			{Label: "Upload", Values: []float64{20, 24, 22, 26, 24}, Role: "violet"},
		},
	})
	for _, want := range []string{
		`<svg`, `role="img"`, `aria-label="Download and upload, last minute"`,
		`verso-chart--full`,
		`viewBox="0 0 620 130"`,
		`verso-chart-grid`, // full size draws gridlines
		`verso-chart--sky`, `verso-chart--violet`,
		`verso-chart-area`, `fill="url(#vc1-0)"`, `id="vc1-0"`, // series 0 is filled
		`verso-chart-line`,
		`verso-chart-dot`, // full size marks the current value
	} {
		if !strings.Contains(got, want) {
			t.Errorf("full chart missing %q:\n%s", want, got)
		}
	}
	// The second series is a line only — the chart has exactly one gradient/area.
	if n := strings.Count(got, "verso-chart-area"); n != 1 {
		t.Errorf("want exactly one filled area, got %d:\n%s", n, got)
	}
}

// TestRenderChartAxis: a full chart with Axis on draws a labelled value scale — the top
// of the range carries the unit, the middle is the bare number — plus the window
// captions, the right one right-anchored.
func TestRenderChartAxis(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size: "full", Max: 400, Unit: "Mbps", Axis: true, AxisStart: "60s ago", AxisEnd: "now",
		Series: []ChartSeries{{Values: []float64{100, 200, 300}, Role: "sky", Fill: true}},
	})
	for _, want := range []string{
		"verso-chart-plot",
		`style="top:0.0%"`, ">400 Mbps<", // top of the scale, with the unit
		`style="top:50.0%"`, ">200<", // the middle level, number only
		"verso-chart-xl-start", ">60s ago<",
		"verso-chart-xl-end", ">now<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("axis chart missing %q:\n%s", want, got)
		}
	}
	// A spark never grows an axis or a plot wrapper, even if the fields are set.
	spark := render(t, r, &Chart{Size: "spark", Axis: true, Unit: "Mbps", Series: []ChartSeries{{Values: []float64{1, 2, 3}}}})
	if strings.Contains(spark, "verso-chart-yl") || strings.Contains(spark, "verso-chart-plot") {
		t.Errorf("spark must stay bare, got axis/plot chrome:\n%s", spark)
	}
}

// TestRenderChartSpark: the per-port sparkline — bare and compact, no axes, no dot.
func TestRenderChartSpark(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size:   "spark",
		Series: []ChartSeries{{Values: []float64{10, 40, 20, 55, 30}, Role: "sky", Fill: true}},
	})
	for _, want := range []string{`verso-chart--spark`, `viewBox="0 0 210 34"`, `verso-chart-line`} {
		if !strings.Contains(got, want) {
			t.Errorf("spark chart missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verso-chart-grid") {
		t.Errorf("spark chart must not draw gridlines:\n%s", got)
	}
	if strings.Contains(got, "verso-chart-dot") {
		t.Errorf("spark chart must not draw an endpoint dot:\n%s", got)
	}
}

// TestRenderChartIdle: an idle chart draws every series in the calm grey, whatever role
// each series asked for.
func TestRenderChartIdle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size:   "spark",
		Idle:   true,
		Series: []ChartSeries{{Values: []float64{1, 1, 1, 1}, Role: "sky", Fill: true}},
	})
	if !strings.Contains(got, "verso-chart--idle") {
		t.Errorf("idle chart should use the idle palette:\n%s", got)
	}
	if strings.Contains(got, "verso-chart--sky") {
		t.Errorf("idle chart must not keep the series' own role colour:\n%s", got)
	}
}

// TestChartGradientIdsUnique: two charts rendered by one renderer get distinct gradient
// ids, so their <defs> never collide on a shared page.
func TestChartGradientIdsUnique(t *testing.T) {
	r := newRenderer(t)
	spec := &Chart{Size: "full", Series: []ChartSeries{{Values: []float64{1, 2, 3}, Fill: true}}}
	first := render(t, r, spec)
	second := render(t, r, spec)
	if !strings.Contains(first, `id="vc1-0"`) {
		t.Errorf("first chart should own gradient vc1-0:\n%s", first)
	}
	if !strings.Contains(second, `id="vc2-0"`) {
		t.Errorf("second chart should own gradient vc2-0:\n%s", second)
	}
}

// TestChartScalesToMax: the scale top (fixed Max) sets where the peak lands, so the same
// values plot at different heights under different maxima.
func TestChartScalesToMax(t *testing.T) {
	spec := func(max float64) *Chart {
		return &Chart{Size: "full", Max: max, Series: []ChartSeries{{Values: []float64{0, 100}}}}
	}
	// Fresh renderers keep the gradient sequence equal, isolating the geometry.
	full := render(t, newRenderer(t), spec(100)) // 100 fills the scale → peak at the top (y=12)
	half := render(t, newRenderer(t), spec(200)) // 100 is half the scale → peak mid-height (y=65)
	if !strings.Contains(full, "620.0 12.0") {
		t.Errorf("value at Max should plot at the top:\n%s", full)
	}
	if !strings.Contains(half, "620.0 65.0") {
		t.Errorf("value at half of Max should plot mid-height:\n%s", half)
	}
}

// TestChartSkipsShortSeries: a series with fewer than two points can't be a line — it is
// skipped, not a panic or a broken path.
func TestChartSkipsShortSeries(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{Size: "spark", Series: []ChartSeries{{Values: []float64{42}}}})
	if !strings.Contains(got, "<svg") {
		t.Errorf("a short series should still yield an svg frame:\n%s", got)
	}
	if strings.Contains(got, "verso-chart-line") {
		t.Errorf("a one-point series must draw no line:\n%s", got)
	}
}

// TestRenderChartReadout: a full chart with a status + rates readout draws the status
// headline (with a live dot), the meta line, and each series' current value coloured to
// its line.
func TestRenderChartReadout(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size: "full", Unit: "Mbps", Title: "Online", Live: true, Note: "PPPoE · 15h 20m",
		Series: []ChartSeries{
			{Label: "Download", Value: "300", Values: []float64{1, 2, 3}, Role: "sky", Fill: true},
			{Label: "Upload", Value: "24", Values: []float64{1, 1, 1}, Role: "violet"},
		},
	})
	for _, want := range []string{
		"verso-chart-hero", "verso-chart-readout", "verso-chart-plot",
		"verso-chart-status", ">Online<", "verso-live-dot", "PPPoE · 15h 20m",
		"verso-chart-rate verso-chart--sky", ">300<", "Mbps", "Download",
		"verso-chart-rate verso-chart--violet", ">24<", "Upload",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("readout chart missing %q:\n%s", want, got)
		}
	}
	// A spark ignores the readout entirely, even with the fields set.
	spark := render(t, r, &Chart{Size: "spark", Title: "Online", Series: []ChartSeries{{Value: "5", Values: []float64{1, 2}}}})
	if strings.Contains(spark, "verso-chart-readout") {
		t.Errorf("spark must not render a readout:\n%s", spark)
	}
}

// TestChartPanel: the fixed-height detail plot — four quarter gridlines with
// their values (unit on the topmost), the stretch mode that lets CSS own the
// height, and the live handle + role hooks the stream updates through.
func TestChartPanel(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Chart{
		Size: "panel", Name: "aa:bb", Axis: true, Unit: "Mbps", Max: 400,
		Series: []ChartSeries{
			{Values: []float64{10, 200, 100}, Role: "sky", Fill: true},
			{Values: []float64{1, 5, 2}, Role: "violet"},
		},
	})
	for _, want := range []string{
		`data-verso-chart="aa:bb"`, `preserveAspectRatio="none"`, "verso-chart--panel",
		`data-verso-chart-role="sky"`, `data-verso-chart-role="violet"`,
		">400 Mbps<", ">300<", ">200<", ">100<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("panel chart missing %q:\n%s", want, got)
		}
	}
	// Four value gridlines plus the value-0 baseline.
	if lines := strings.Count(got, "<line "); lines != 5 {
		t.Errorf("panel lines = %d, want 5 (4 gridlines + baseline)", lines)
	}
	if !strings.Contains(got, "verso-chart-baseline") {
		t.Errorf("panel chart should draw a value-0 baseline")
	}
	// Every value label rides across its line, and the time captions sit in
	// their own row below the plot rather than overlaid inside it.
	if n := strings.Count(got, "verso-chart-yl--mid"); n != 4 {
		t.Errorf("panel mid-riding labels = %d, want 4", n)
	}
	withTimes := render(t, r, &Chart{
		Size: "panel", Axis: true, AxisStart: "60s ago", AxisEnd: "now",
		Series: []ChartSeries{{Values: []float64{1, 2}, Role: "sky"}},
	})
	if !strings.Contains(withTimes, "verso-chart-xrow") || strings.Contains(withTimes, "verso-chart-xl-start") {
		t.Errorf("panel time captions should sit in the row below:\n%s", withTimes)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/units"
)

// TestAChartIsSizedByItsPlace: a page's card rules its plot in 32px squares,
// an Overview card or a drawer in 24px ones; the plot stands on its own line,
// never a pixel up.
func TestAChartIsSizedByItsPlace(t *testing.T) {
	series := []ChartSeries{{Values: []float64{1, 2, 3}}}
	for _, tc := range []struct {
		place ChartPlace
		want  string
	}{
		{PlacePage, `data-chart-place="page"`},
		{PlaceCompact, `data-chart-place="compact"`},
	} {
		got := render(t, newRenderer(t), &Chart{Place: tc.place, Span: 24 * time.Hour, Series: series})
		if !strings.Contains(got, tc.want) {
			t.Errorf("place %v: missing %s:\n%s", tc.place, tc.want, got)
		}
		if strings.Contains(got, "-mt-px") {
			t.Errorf("place %v: the plot is shifted a pixel up:\n%s", tc.place, got)
		}
	}
}

var tickLabel = regexp.MustCompile(`data-verso-chart-tick[^>]*>([^<]*)<`)

// ticks are the labels of the time axis that cuts the window into steps, left
// to right.
func ticks(html string, steps string) []string {
	_, axis, _ := strings.Cut(html, `data-chart-axis="`+steps+`"`)
	axis, _, _ = strings.Cut(axis, "</div>")
	var out []string
	for _, m := range tickLabel.FindAllStringSubmatch(axis, -1) {
		out = append(out, m[1])
	}
	return out
}

// TestAWindowOffersTheStepsItCutsInto: a window offers each cut whose step is
// a whole number of its unit, in order of preference (quarters, sixths,
// thirds), each its own labels, how long ago each step stands as every
// duration is written, the last "now"; a window no such step cuts keeps its
// quarters. The page shows the one that gives the widest plot.
func TestAWindowOffersTheStepsItCutsInto(t *testing.T) {
	series := []ChartSeries{{Values: []float64{1, 2, 3}}}
	for _, tc := range []struct {
		span  time.Duration
		steps string
		axes  map[string]string
	}{
		{24 * time.Hour, "4,6,3", map[string]string{
			"4": "24\U0000202fh|18\U0000202fh|12\U0000202fh|6\U0000202fh|now", "6": "24\U0000202fh|20\U0000202fh|16\U0000202fh|12\U0000202fh|8\U0000202fh|4\U0000202fh|now", "3": "24\U0000202fh|16\U0000202fh|8\U0000202fh|now",
		}},
		{30 * 24 * time.Hour, "6,3", map[string]string{"6": "30\U0000202fd|25\U0000202fd|20\U0000202fd|15\U0000202fd|10\U0000202fd|5\U0000202fd|now", "3": "30\U0000202fd|20\U0000202fd|10\U0000202fd|now"}},
		{7 * 24 * time.Hour, "4", map[string]string{"4": "7\U0000202fd|5\U0000202fd 6\U0000202fh|3\U0000202fd 12\U0000202fh|1\U0000202fd 18\U0000202fh|now"}},
	} {
		got := render(t, newRenderer(t), &Chart{Span: tc.span, Series: series})
		if !strings.Contains(got, `data-chart-steps="`+tc.steps+`"`) {
			t.Errorf("span %v does not offer %s:\n%s", tc.span, tc.steps, got)
		}
		for steps, want := range tc.axes {
			if axis := strings.Join(ticks(got, steps), "|"); axis != want {
				t.Errorf("span %v in %s steps: axis %q, want %q", tc.span, steps, axis, want)
			}
		}
	}
}

// TestAChartsLegendSaysItsSeriesFigureAndPeak: over the plot, the series'
// packet in its hue, its name, its figure in its unit and what the figure is,
// and at the plot's right edge the window's peak, which a dotted line marks in
// the plot; the scale and figures are written in the chart's unit.
func TestAChartsLegendSaysItsSeriesFigureAndPeak(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{
		Unit: units.UnitCelsius, Span: 24 * time.Hour,
		Series: []ChartSeries{{
			Values: []float64{46, 58, 48}, Role: "denim", Fill: true,
			Legend: &ChartLegend{Name: "CPU", Figure: 48.66, Caption: "on average", Peak: 59},
		}},
	})
	for _, want := range []string{
		"data-verso-chart-legend", `bg-denim`, ">CPU<", ">48.7\U0000202f°C<", ">on average<", ">peak<", ">59.0\U0000202f°C<",
		">80<", ">60<", "data-verso-chart-unit>°C<",
		// 59 °C under a top of 80 lands at 128 - 59/80·128 on a page's 128px plot.
		`<line class="verso-chart-peak verso-chart-peak--denim" x1="0" x2="620" y1="33.6" y2="33.6"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chart missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "data-verso-chart-legend") > strings.Index(got, "<svg") {
		t.Error("the legend stands under the plot")
	}
	if strings.Index(got, "verso-chart-peak") > strings.Index(got, "verso-chart-series") {
		t.Error("the peak line is drawn over the curve")
	}
}

// TestAChartWithoutALegendDrawsNone: a series with no legend has no legend
// line and no peak mark.
func TestAChartWithoutALegendDrawsNone(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{Span: time.Hour, Series: []ChartSeries{{Values: []float64{1, 2}}}})
	if strings.Contains(got, "data-verso-chart-legend") || strings.Contains(got, "verso-chart-peak") {
		t.Errorf("a series with no legend drew one:\n%s", got)
	}
}

// TestAQuietLineRidesOverTheBottomBorder: the plot is drawn in its own pixels
// (128 on a page, 96 in a card), and a zero reading stands one pixel over the
// bottom border instead of on it, while its wash still reaches the border.
func TestAQuietLineRidesOverTheBottomBorder(t *testing.T) {
	for place, want := range map[ChartPlace]struct{ viewBox, line, area string }{
		PlacePage:    {`viewBox="0 0 620 128"`, `d="M0.0 127.0 `, `M0.0 128.0 L0.0 127.0 `},
		PlaceCompact: {`viewBox="0 0 620 96"`, `d="M0.0 95.0 `, `M0.0 96.0 L0.0 95.0 `},
	} {
		got := render(t, newRenderer(t), &Chart{Place: place, Span: time.Hour, Series: []ChartSeries{{Values: []float64{0, 0, 5}, Fill: true}}})
		for _, w := range []string{want.viewBox, want.line, want.area} {
			if !strings.Contains(got, w) {
				t.Errorf("place %v missing %q:\n%s", place, w, got)
			}
		}
	}
}

// TestTheLiveMinuteIsLabelledAtItsSteps: the grid stands still under a live
// curve, so the minute is named on its lines as any window is, in 15 s, 10 s
// or 20 s.
func TestTheLiveMinuteIsLabelledAtItsSteps(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{Span: time.Minute, Series: []ChartSeries{{Values: []float64{1, 2}}}})
	for steps, want := range map[string]string{"4": "60\U0000202fs|45\U0000202fs|30\U0000202fs|15\U0000202fs|now", "6": "60\U0000202fs|50\U0000202fs|40\U0000202fs|30\U0000202fs|20\U0000202fs|10\U0000202fs|now", "3": "60\U0000202fs|40\U0000202fs|20\U0000202fs|now"} {
		if axis := strings.Join(ticks(got, steps), "|"); axis != want {
			t.Errorf("the minute in %s steps: %q, want %q", steps, axis, want)
		}
	}
}

// TestRenderChart: the plot stretches edge to edge, its peak sits under a
// round top (100 under 120, counted in thirties), the scale's figures ride the
// quarter lines bare, its unit on its baseline, and the window's quarters are
// named under it.
func TestRenderChart(t *testing.T) {
	got := render(t, newRenderer(t), &Chart{
		Span: time.Minute, Label: "Traffic",
		Series: []ChartSeries{
			{Values: []float64{0, 100}, Role: "emerald", Fill: true},
			{Values: []float64{0, 50}, Role: "violet"},
		},
	})
	for _, want := range []string{
		`preserveAspectRatio="none"`, `aria-label="Traffic"`, `data-chart-padding="0"`,
		"verso-chart--emerald", "verso-chart--violet",
		"620.0 21.3", // 100 under a top of 120 lands at 128 - 100/120·128
		">120<", ">90<", ">60<", ">30<", `style="top:0.0%"`, `style="top:75.0%"`,
		`style="top:100%" data-verso-chart-unit>Mb/s<`,
		">60\U0000202fs<", ">30\U0000202fs<", ">now<",
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
		`<rect class="verso-chart-mark--outage" x="155.0" y="0" width="155.0" height="128"><title>Internet down 03:12–03:20</title></rect>`,
		`<line class="verso-chart-mark--boot" x1="465.0" x2="465.0" y1="0" y2="128" vector-effect="non-scaling-stroke"><title>Router started at 04:00</title></line>`,
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/units"
)

// Chart is a reading over time: lines, optionally filled as areas, drawn
// server-side to static SVG over square cells, four cells tall so every rule
// is a quarter line, its value labels in a gutter on the plot's right and the
// window's quarters named under it, so nothing is laid over the curve. The
// plot's width is whole quarters of cells (the CSS rounds it), so each time
// label stands on its line. This SVG is the first paint; the live layer
// (verso-chart.js) glides the same series without changing the markup.
//
// Colours are set by CSS classes, never SVG presentation attributes: var() resolves in
// CSS properties but not inside SVG attributes, so a .verso-chart-* class carries the
// palette while geometry (viewBox, d) stays on the element.
//
// The values are rates in Mbit/s, on a round scale over their tallest point
// (units.RateScale): the topmost value label is a rate in full ("80 Mb/s"),
// the others figures in its unit (60, 40, 20).
type Chart struct {
	Series []ChartSeries // drawn back to front
	Label  string        // accessible description of the whole plot
	// Place is where the chart stands, which sizes its cells.
	Place ChartPlace
	// Unit is what the values are counted in, which writes the scale and the
	// legend's figures: Mbit/s by default, or °C.
	Unit units.Unit
	// Span is how far back the window reaches; its quarters are named under
	// the plot (24 h · 18 h · 12 h · 6 h · now).
	Span time.Duration
	// NoAxis leaves the time axis off: a lane standing over another, whose
	// axis serves both.
	NoAxis bool
	// Marks are what happened over the window (ADR-019), drawn behind the
	// curves.
	Marks []ChartMark
}

// ChartPlace is where a chart stands: on a page's card, its cells 32px, or in
// an Overview card or a drawer, its cells 24px.
type ChartPlace int

const (
	PlacePage ChartPlace = iota
	PlaceCompact
)

func (p ChartPlace) String() string {
	if p == PlaceCompact {
		return "compact"
	}
	return "page"
}

// ChartMark is one event on a chart's time axis. From and To are fractions of
// the width: an "outage" or a stretch of packet "loss" is a band between them,
// a "boot" a rule at From.
// Label names it for the pointer, already composed.
type ChartMark struct {
	Kind     string
	From, To float64
	Label    string
}

// ChartSeries is one line on a chart.
type ChartSeries struct {
	Label  string    // series name, for the plot's description
	Values []float64 // the points, left to right
	Role   string    // palette slot: violet | emerald | denim | amethyst
	Fill   bool      // fill the area under the line, not only the line
	// Legend, when set, says the series over the plot and marks its peak.
	Legend *ChartLegend
}

// ChartLegend is what a series' legend line says, as the Overview's facts are
// said: its name (a term, in Meta), its figure in the chart's unit (in Ink)
// and what the figure is, and at the plot's right edge the window's peak,
// which a dotted line also marks in the plot. The name is shown as given: a
// caller translates a word and leaves a profile's name as it is.
type ChartLegend struct {
	Name    string
	Figure  float64
	Caption string // "now" for a live window, "on average" for a past one
	Peak    float64
}

func (*Chart) isWidget() {}

func (*Chart) children() []Widget { return nil }

// The plot's viewBox. The CSS fixes the height and stretches the width, so the
// curve meets both edges at any column width.
const chartW, chartH = 620.0, 240.0

// chartMarkMinWidth is the narrowest an outage band draws, in plot units:
// about three pixels at the overview's width.
const chartMarkMinWidth = 1.5

type chartView struct {
	Label   string
	W, H    float64
	YLabels []chartYLabel
	Ticks   []chartTick
	Series  []chartSeriesView
	Place   string
	Marks   []chartMarkView
	Legends []chartLegendView
	Peaks   []chartPeakView
}

// chartLegendView is a legend line, its figures already written in the
// chart's unit.
type chartLegendView struct {
	Hue, Name, Figure, Caption, Peak string
}

// chartPeakView is a peak's dotted line, at Y in the plot's own units.
type chartPeakView struct {
	Role string
	Y    float64
}

// chartTick is one label on the time axis, at Pct of the plot's width; the
// first stands from its line rightward, the last leftward, the rest centred.
type chartTick struct {
	Text  string
	Pct   float64
	Align string // "start" | "center" | "end"
}

// chartMarkView is a mark in the plot's own units.
type chartMarkView struct {
	Kind     string
	X, Width float64
	Label    string
}

// chartYLabel is one value label, positioned as a percentage of the plot height
// so it stays on its line however the plot stretches.
type chartYLabel struct {
	TopPct float64
	Text   string
}

type chartSeriesView struct {
	Role   string
	GradID string // unique per render, so multiple charts' gradients never collide
	Area   string // area path, empty when the series is not filled
	Line   string
}

func (c *Chart) renderInto(r *Renderer, out io.Writer, _ string) error {
	// Scale to a round top over the tallest point across every series and
	// every peak a legend marks, in the chart's unit (units.Scale): its labels
	// read as counted, and it never divides by zero.
	peak := 0.0
	for _, s := range c.Series {
		for _, v := range s.Values {
			if !math.IsNaN(v) {
				peak = max(peak, v)
			}
		}
		if s.Legend != nil && !math.IsNaN(s.Legend.Peak) {
			peak = max(peak, s.Legend.Peak)
		}
	}
	nums := r.numbers()
	top, labels := nums.Scale(c.Unit, peak)

	view := chartView{Label: c.Label, W: chartW, H: chartH, Place: c.Place.String()}
	if !c.NoAxis {
		view.Ticks = c.ticks(r)
	}
	for _, s := range c.Series {
		if s.Legend == nil {
			continue
		}
		l := s.Legend
		view.Legends = append(view.Legends, chartLegendView{
			Hue: chartHue(s.Role), Name: l.Name, Caption: r.tr(l.Caption),
			Figure: nums.Reading(c.Unit, l.Figure), Peak: nums.Reading(c.Unit, l.Peak),
		})
		view.Peaks = append(view.Peaks, chartPeakView{Role: s.Role, Y: chartH - l.Peak/top*chartH})
	}
	for _, m := range c.Marks {
		view.Marks = append(view.Marks, chartMarkView{
			// A brief outage keeps a width the eye can find.
			Kind: m.Kind, X: m.From * chartW, Width: max(chartMarkMinWidth, (m.To-m.From)*chartW), Label: m.Label,
		})
	}
	// The quarter lines each carry their value — HTML, so it renders at a fixed
	// size instead of scaling with the viewBox.
	for i, frac := range units.AxisFractions {
		view.YLabels = append(view.YLabels, chartYLabel{TopPct: (1 - frac) * 100, Text: labels[i]})
	}

	seq := r.seq.chart.Add(1)
	for i, s := range c.Series {
		if len(s.Values) < 2 {
			continue // a line needs at least two points
		}
		var line, area strings.Builder
		for _, run := range chartRuns(chartPoints(s.Values, chartW, chartH, top)) {
			line.WriteString(chartCurve(run))
			area.WriteString(chartCurveArea(run, chartH))
		}
		if line.Len() == 0 {
			continue
		}
		sv := chartSeriesView{Role: s.Role, Line: line.String()}
		if s.Fill {
			sv.GradID = fmt.Sprintf("vc%d-%d", seq, i)
			sv.Area = area.String()
		}
		view.Series = append(view.Series, sv)
	}
	return r.execute(out, "chart.html.tmpl", view)
}

// chartHue is the packet's colour for a series' palette slot.
func chartHue(role string) string {
	switch role {
	case "denim":
		return "bg-denim"
	case "emerald":
		return "bg-green"
	default:
		return "bg-amethyst"
	}
}

// ticks are the time axis's labels: the window's quarters, live or past, each
// how long ago it stands as every duration is written, counted in no unit
// larger than a quarter reaches (units.DurationWithin: "24 h · 18 h · 12 h ·
// 6 h", "60 s · 45 s · 30 s · 15 s"), the last "now". The grid stands still
// under a live curve, so each label keeps its line.
func (c *Chart) ticks(r *Renderer) []chartTick {
	now := r.tr("now")
	seconds := int64(c.Span / time.Second)
	quarter := seconds / 4
	out := make([]chartTick, 0, 5)
	for q := range int64(4) {
		align := "center"
		if q == 0 {
			align = "start"
		}
		out = append(out, chartTick{units.DurationWithin(quarter*(4-q), quarter).String(), float64(q) * 25, align})
	}
	return append(out, chartTick{now, 100, "end"})
}

// chartPoints maps the values to plot coordinates: x spreads them evenly across the
// width, y maps each value into the height (the top of the scale is top).
func chartPoints(vals []float64, w, h, top float64) [][2]float64 {
	pts := make([][2]float64, len(vals))
	for i, v := range vals {
		pts[i] = [2]float64{float64(i) / float64(len(vals)-1) * w, h - v/top*h}
	}
	return pts
}

// chartSegments appends the cubic-bezier segments of a smooth curve through the points
// (a Catmull-Rom spline, tension 1/6): each segment eases into the next so the line
// reads as a curve, not a run of straight hops. Constraining each control point's y
// coordinate to its segment endpoints prevents the spline from inventing negative
// traffic between non-negative samples. The opening "M" is written by the caller.
func chartSegments(b *strings.Builder, pts [][2]float64) {
	for i := 0; i < len(pts)-1; i++ {
		p0 := pts[i]
		if i > 0 {
			p0 = pts[i-1]
		}
		p1, p2 := pts[i], pts[i+1]
		p3 := p2
		if i+2 < len(pts) {
			p3 = pts[i+2]
		}
		lo, hi := min(p1[1], p2[1]), max(p1[1], p2[1])
		c1y := max(lo, min(hi, p1[1]+(p2[1]-p0[1])/6))
		c2y := max(lo, min(hi, p2[1]-(p3[1]-p1[1])/6))
		fmt.Fprintf(b, " C%.1f %.1f %.1f %.1f %.1f %.1f",
			p1[0]+(p2[0]-p0[0])/6, c1y,
			p2[0]-(p3[0]-p1[0])/6, c2y,
			p2[0], p2[1])
	}
}

// chartCurve is the smooth line through the points.
func chartCurve(pts [][2]float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "M%.1f %.1f", pts[0][0], pts[0][1])
	chartSegments(&b, pts)
	return b.String()
}

// chartCurveArea is the same smooth curve closed down to the baseline and back — the
// filled shape under the line, from the run's first point to its last.
func chartCurveArea(pts [][2]float64, h float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "M%.1f %.1f L%.1f %.1f", pts[0][0], h, pts[0][0], pts[0][1])
	chartSegments(&b, pts)
	fmt.Fprintf(&b, " L%.1f %.1f Z", pts[len(pts)-1][0], h)
	return b.String()
}

// chartRuns splits the points at gaps (a NaN value, nothing recorded) into the
// runs a line can be drawn through; a lone point between gaps is no line.
func chartRuns(pts [][2]float64) [][][2]float64 {
	var runs [][][2]float64
	start := 0
	for i := 0; i <= len(pts); i++ {
		if i < len(pts) && !math.IsNaN(pts[i][1]) {
			continue
		}
		if i-start >= 2 {
			runs = append(runs, pts[start:i])
		}
		start = i + 1
	}
	return runs
}

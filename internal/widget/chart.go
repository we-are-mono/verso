// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/we-are-mono/verso/internal/units"
)

// Chart is the overview's traffic plot: lines, optionally filled as areas, drawn
// server-side to static SVG edge to edge on the notebook grid, with value labels
// riding the quarter lines and the window captioned in the bottom corners. This
// SVG is the first paint; the live layer (verso-stream.js) glides the same series
// without changing the markup.
//
// Colours are set by CSS classes, never SVG presentation attributes: var() resolves in
// CSS properties but not inside SVG attributes, so a .verso-chart-* class carries the
// palette while geometry (viewBox, d) stays on the element.
//
// The values are rates in Mbit/s. The topmost value label is a rate in full
// ("1.2 Gb/s"), the others figures in its unit, written as every rate is
// (internal/units).
type Chart struct {
	Series []ChartSeries // drawn back to front
	Label  string        // accessible description of the whole plot
	// AxisStart and AxisEnd caption the window's two ends ("60 s ago", "now").
	AxisStart string
	AxisEnd   string
	// Short stands the plot at 180px rather than 240px, nine cells of the grid
	// rather than twelve; the viewBox stretches, so the curve is the same.
	Short bool
	// Paper stands the plot on Paper rather than the notebook's sand (an
	// Overview card): its labels sit on Paper too.
	Paper bool
	// Marks are what happened over the window (ADR-019), drawn behind the
	// curves.
	Marks []ChartMark
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
	Role   string    // palette slot: violet | emerald | denim
	Fill   bool      // fill the area under the line, not only the line
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
	XStart  string
	XEnd    string
	Series  []chartSeriesView
	Short   bool
	Paper   bool
	Marks   []chartMarkView
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
	// Auto-scale to the tallest point across every series with a little headroom
	// so the peak never touches the top. Never divide by zero.
	top := 0.0
	for _, s := range c.Series {
		for _, v := range s.Values {
			if !math.IsNaN(v) {
				top = max(top, v)
			}
		}
	}
	top *= 1.15
	if top <= 0 {
		top = 1
	}

	view := chartView{Label: c.Label, W: chartW, H: chartH, XStart: c.AxisStart, XEnd: c.AxisEnd, Short: c.Short, Paper: c.Paper}
	for _, m := range c.Marks {
		view.Marks = append(view.Marks, chartMarkView{
			// A brief outage keeps a width the eye can find.
			Kind: m.Kind, X: m.From * chartW, Width: max(chartMarkMinWidth, (m.To-m.From)*chartW), Label: m.Label,
		})
	}
	// Four barely-there dividing lines at the quarters of the range, each
	// carrying its value — HTML overlaid on the plot, so it renders at a fixed
	// size instead of scaling with the viewBox.
	labels := r.numbers().RateAxis(top*1e6, units.AxisFractions)
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

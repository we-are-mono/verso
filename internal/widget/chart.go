// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
	"strings"
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
type Chart struct {
	Series []ChartSeries // drawn back to front
	Label  string        // accessible description of the whole plot
	// Unit rides the topmost value label; AxisStart and AxisEnd caption the
	// window's two ends ("60 s ago", "now").
	Unit      string
	AxisStart string
	AxisEnd   string
	// Short stands the plot at 180px rather than 240px, nine cells of the grid
	// rather than twelve; the viewBox stretches, so the curve is the same.
	Short bool
}

// ChartSeries is one line on a chart.
type ChartSeries struct {
	Label  string    // series name, for the plot's description
	Values []float64 // the points, left to right
	Role   string    // palette slot: violet | emerald
	Fill   bool      // fill the area under the line, not only the line
}

func (*Chart) isWidget() {}

func (*Chart) children() []Widget { return nil }

// The plot's viewBox. The CSS fixes the height and stretches the width, so the
// curve meets both edges at any column width.
const chartW, chartH = 620.0, 240.0

type chartView struct {
	Label   string
	W, H    float64
	YLabels []chartYLabel
	XStart  string
	XEnd    string
	Series  []chartSeriesView
	Short   bool
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
			top = max(top, v)
		}
	}
	top *= 1.15
	if top <= 0 {
		top = 1
	}

	view := chartView{Label: c.Label, W: chartW, H: chartH, XStart: c.AxisStart, XEnd: c.AxisEnd, Short: c.Short}
	// Four barely-there dividing lines at the quarters of the range, each
	// carrying its value — HTML overlaid on the plot, so it renders at a fixed
	// size instead of scaling with the viewBox.
	for i, frac := range []float64{1, 0.75, 0.5, 0.25} {
		text := chartAxisValue(top * frac)
		if i == 0 && c.Unit != "" {
			text += " " + c.Unit
		}
		view.YLabels = append(view.YLabels, chartYLabel{TopPct: (1 - frac) * 100, Text: text})
	}

	seq := r.seq.chart.Add(1)
	for i, s := range c.Series {
		if len(s.Values) < 2 {
			continue // a line needs at least two points
		}
		pts := chartPoints(s.Values, chartW, chartH, top)
		sv := chartSeriesView{Role: s.Role, Line: chartCurve(pts)}
		if s.Fill {
			sv.GradID = fmt.Sprintf("vc%d-%d", seq, i)
			sv.Area = chartCurveArea(pts, chartW, chartH)
		}
		view.Series = append(view.Series, sv)
	}
	return r.execute(out, "chart.html.tmpl", view)
}

// chartAxisValue renders a gridline's value compactly — whole numbers from 10
// up, one decimal below.
func chartAxisValue(v float64) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f", v)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0")
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
// filled shape under the line.
func chartCurveArea(pts [][2]float64, w, h float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "M0 %.1f L%.1f %.1f", h, pts[0][0], pts[0][1])
	chartSegments(&b, pts)
	fmt.Fprintf(&b, " L%.1f %.1f Z", w, h)
	return b.String()
}

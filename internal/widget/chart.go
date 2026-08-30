// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
	"strings"
)

// Chart is a generic time-series plot — a line, optionally filled as an area, drawn
// server-side to static SVG from a list of numbers. One widget powers both the hero
// throughput graph (size "full": faint gridlines and an endpoint dot) and the per-port
// sparklines (size "spark": bare and compact). It names no metric; it plots whatever
// values it is given, the same way an HTML element would.
//
// The value is the schema, not the pixels: a plugin emits only data (series, size,
// scale) and the shell owns the rendering (ADR-005), which keeps the renderer
// swappable. This static SVG is the first paint; the live layer (smooth streaming via a
// shell-owned component) hydrates the same series without changing the contract, so the
// static work is never thrown away.
//
// Colours are set by CSS classes, never SVG presentation attributes: var() resolves in
// CSS properties but not inside SVG attributes, so a .verso-chart-* class carries the
// palette while geometry (viewBox, d, cx/cy) stays on the element.
type Chart struct {
	Series []ChartSeries `json:"series"` // 1–2 series, drawn back to front
	Size   string        `json:"size"`   // "full" (hero) | "panel" (fixed-height detail) | "spark" (compact, the default)
	Max    float64       `json:"max"`    // fixed top of scale; 0 auto-scales to the data
	Label  string        `json:"label"`  // accessible description of the whole plot
	Idle   bool          `json:"idle"`   // draw calm grey — a quiet or empty source
	// Name is a stable handle for a live chart: the rendered SVG carries it
	// (plus per-series role hooks) so the shell's client script can stream
	// fresh series into the drawn paths. A nameless chart is static.
	Name string `json:"name,omitempty"`

	// Axis draws a value scale on a "full" chart: labelled horizontal gridlines at the
	// top and middle of the range (Unit rides the top label), plus optional captions at
	// the left and right ends of the window. Sparklines leave it off and stay bare.
	Unit      string `json:"unit"`
	Axis      bool   `json:"axis"`
	AxisStart string `json:"axis_start"`
	AxisEnd   string `json:"axis_end"`

	// Title, Live, and Note drive an optional readout beside a "full" chart: a status
	// headline (with a pulsing live dot when Live), a meta line under it, and the
	// current value of each series (ChartSeries.Value). Together they turn the graph
	// into a live metric hero. Sparklines ignore all of it and stay bare.
	Title string `json:"title"`
	Live  bool   `json:"live"`
	Note  string `json:"note"`
}

// ChartSeries is one line on the plot.
type ChartSeries struct {
	Label  string    `json:"label"`  // series name (the readout label + the plot's description)
	Value  string    `json:"value"`  // current value for the readout (e.g. "300"); shown with Unit
	Values []float64 `json:"values"` // the points, left to right
	Role   string    `json:"role"`   // palette slot: sky (default) | violet | emerald | amber
	Fill   bool      `json:"fill"`   // fill the area under the line, not only the line
}

func (*Chart) isWidget() {}

// chartDims are the drawing constants for a size: the viewBox, the vertical padding
// that keeps peaks and troughs off the edges, whether faint gridlines are drawn, and
// the endpoint-dot radius (0 = no dot). "full" is the hero graph; "spark" is a bare
// sparkline sized for a card.
type chartDims struct {
	W, H, Pad float64
	Grid      bool
	Dot       float64
}

func chartDimsFor(size string) chartDims {
	switch size {
	case "full":
		return chartDims{W: 620, H: 130, Pad: 12, Grid: true, Dot: 3}
	case "panel":
		// The fixed-height detail plot (12.5rem via CSS): the viewBox height
		// matches the rendered height, so strokes draw at their nominal width.
		return chartDims{W: 620, H: 200, Pad: 8, Grid: false, Dot: 3}
	}
	return chartDims{W: 210, H: 34, Pad: 3, Grid: false, Dot: 0}
}

type chartView struct {
	Label    string
	Name     string // live handle (data-verso-chart), empty for a static chart
	Class    string // size class on the svg root: verso-chart--full | verso-chart--panel | verso-chart--spark
	Stretch  bool   // preserveAspectRatio="none": the CSS fixes the height, the width flexes
	W, H     float64
	GridYs   []float64
	Baseline float64 // y of the value-0 line; 0 = none (full/panel draw it)
	HasAxis  bool    // wrap the plot so the HTML axis labels can overlay it
	HTMLDots []chartHTMLDot
	YLabels  []chartYLabel
	XStart   string
	XEnd     string
	Series   []chartSeriesView
	Readout  bool // draw the status/rates panel beside the graph
	Title    string
	Live     bool
	Note     string
	Rates    []chartRate
}

// chartYLabel is one value label riding a gridline, positioned by percent of
// the plot's height (inline style — SVG text would scale with the viewBox).
type chartYLabel struct {
	TopPct float64
	Mid    bool // vertically centre on the line (the topmost label hangs below it)
	Text   string
}

// chartHTMLDot is an endpoint marker drawn as an HTML overlay (not an SVG circle) so
// it stays round on a stretched plot, where preserveAspectRatio="none" would squash a
// circle into an oval. It rides the plot's right edge at the series' last value.
type chartHTMLDot struct {
	RoleClass string
	TopPct    float64
}

// chartRate is one current-value entry in the readout: a value in its unit, coloured to
// its series, with the series label.
type chartRate struct {
	RoleClass string
	Role      string // raw palette slot for the panel readout's swatch class
	Value     string
	Unit      string
	Label     string
}

type chartSeriesView struct {
	RoleClass string // sky | violet | emerald | amber | idle
	Role      string // the raw palette slot, stamped as a hook so a live chart can leave idle
	GradID    string // unique per render, so multiple charts' gradients never collide
	Area      string // area path, empty when the series is not filled
	Line      string
	DotR      float64 // 0 = no endpoint dot
	DotX      string
	DotY      string
}

func (c *Chart) renderInto(r *Renderer, out io.Writer, _ string) error {
	dims := chartDimsFor(c.Size)

	// Scale to the fixed Max, or auto-scale to the tallest point across every series
	// with a little headroom so the peak never touches the top. Never divide by zero.
	max := c.Max
	if max <= 0 {
		for _, s := range c.Series {
			for _, v := range s.Values {
				if v > max {
					max = v
				}
			}
		}
		max *= 1.15
	}
	if max <= 0 {
		max = 1
	}

	class := "verso-chart--spark"
	switch c.Size {
	case "full":
		class = "verso-chart--full"
	case "panel":
		class = "verso-chart--panel"
	}

	view := chartView{Label: c.Label, Name: c.Name, Class: class, W: dims.W, H: dims.H, Stretch: c.Size == "panel"}
	scaleY := func(v float64) float64 { return dims.H - dims.Pad - (v/max)*(dims.H-2*dims.Pad) }
	switch {
	case c.Axis && c.Size == "panel":
		// Four barely-there dividing lines at the quarters of the range, each
		// carrying its value. Labels are HTML overlaid on the plot (not SVG
		// text), so they render at a fixed size instead of scaling.
		view.HasAxis = true
		for i, frac := range []float64{1, 0.75, 0.5, 0.25} {
			y := scaleY(max * frac)
			view.GridYs = append(view.GridYs, y)
			text := chartAxisValue(max * frac)
			if i == 0 && c.Unit != "" {
				text += " " + c.Unit
			}
			// Every label rides across its line — the topmost included.
			view.YLabels = append(view.YLabels, chartYLabel{TopPct: y / dims.H * 100, Mid: true, Text: text})
		}
		view.XStart = c.AxisStart
		view.XEnd = c.AxisEnd
	case c.Axis && c.Size == "full":
		// Value gridlines at the top and middle of the range, labelled likewise.
		view.GridYs = []float64{scaleY(max), scaleY(max / 2)}
		view.HasAxis = true
		top := chartAxisValue(max)
		if c.Unit != "" {
			top += " " + c.Unit
		}
		view.YLabels = []chartYLabel{
			{TopPct: 0, Text: top},
			{TopPct: 50, Mid: true, Text: chartAxisValue(max / 2)},
		}
		view.XStart = c.AxisStart
		view.XEnd = c.AxisEnd
	case dims.Grid:
		view.GridYs = []float64{dims.Pad, (dims.H + dims.Pad) / 2}
	}

	// A value-0 baseline anchors the plot's bottom on the hero and panel charts
	// (a sparkline stays bare).
	if c.Size == "full" || c.Size == "panel" {
		view.Baseline = scaleY(0)
	}

	seq := r.chartSeq.Add(1)
	for i, s := range c.Series {
		if len(s.Values) < 2 {
			continue // a line needs at least two points
		}
		pts := chartPoints(s.Values, dims.W, dims.H, max, dims.Pad)
		sv := chartSeriesView{
			RoleClass: chartRoleClass(s.Role, c.Idle),
			Role:      s.Role,
			Line:      chartCurve(pts),
		}
		if s.Fill {
			sv.GradID = fmt.Sprintf("vc%d-%d", seq, i)
			sv.Area = chartCurveArea(pts, dims.W, dims.H)
		}
		if dims.Dot > 0 {
			lastY := pts[len(pts)-1][1]
			if view.Stretch {
				// A stretched plot squashes an SVG circle; draw the dot as a
				// round HTML overlay at the plot's right edge instead.
				view.HTMLDots = append(view.HTMLDots, chartHTMLDot{RoleClass: sv.RoleClass, TopPct: lastY / dims.H * 100})
			} else {
				sv.DotR = dims.Dot
				sv.DotX = fmt.Sprintf("%.1f", dims.W)
				sv.DotY = fmt.Sprintf("%.1f", lastY)
			}
		}
		view.Series = append(view.Series, sv)
	}

	// A full or panel chart may carry a readout: the current value of each
	// series, coloured to match its line — beside the hero, above the panel
	// (where Note becomes the quiet sampling tag). Sparklines never do.
	if c.Size == "full" || c.Size == "panel" {
		view.Title, view.Live, view.Note = c.Title, c.Live, c.Note
		for _, s := range c.Series {
			if s.Value != "" {
				view.Rates = append(view.Rates, chartRate{
					RoleClass: chartRoleClass(s.Role, c.Idle),
					// The panel readout keys its swatch on identity, not state:
					// the legend still names the lines while the minute is idle.
					Role:  chartRoleClass(s.Role, false),
					Value: s.Value,
					Unit:  c.Unit,
					Label: s.Label,
				})
			}
		}
		view.Readout = view.Title != "" || view.Note != "" || len(view.Rates) > 0
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

// chartRoleClass maps a series role to a palette-slot class the shell owns (ADR-005).
// An idle chart overrides every series to a calm grey.
func chartRoleClass(role string, idle bool) string {
	if idle {
		return "idle"
	}
	switch role {
	case "sky", "violet", "emerald", "amber":
		return role
	default:
		return "sky"
	}
}

// chartPoints maps the values to plot coordinates: x spreads them evenly across the
// width, y maps each value into the padded height (the top of the scale is max).
func chartPoints(vals []float64, w, h, max, pad float64) [][2]float64 {
	pts := make([][2]float64, len(vals))
	for i, v := range vals {
		pts[i] = [2]float64{
			float64(i) / float64(len(vals)-1) * w,
			h - pad - (v/max)*(h-2*pad),
		}
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

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
	Size   string        `json:"size"`   // "full" (hero) | "spark" (compact, the default)
	Max    float64       `json:"max"`    // fixed top of scale; 0 auto-scales to the data
	Label  string        `json:"label"`  // accessible description of the whole plot
	Idle   bool          `json:"idle"`   // draw calm grey — a quiet or empty source

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
	if size == "full" {
		return chartDims{W: 620, H: 130, Pad: 12, Grid: true, Dot: 3}
	}
	return chartDims{W: 210, H: 34, Pad: 3, Grid: false, Dot: 0}
}

type chartView struct {
	Label   string
	Class   string // size class on the svg root: verso-chart--full | verso-chart--spark
	W, H    float64
	GridYs  []float64
	HasAxis bool // wrap the plot so the HTML axis labels can overlay it
	AxisTop string
	AxisMid string
	XStart  string
	XEnd    string
	Series  []chartSeriesView
	Readout bool // draw the status/rates panel beside the graph
	Title   string
	Live    bool
	Note    string
	Rates   []chartRate
}

// chartRate is one current-value entry in the readout: a value in its unit, coloured to
// its series, with the series label.
type chartRate struct {
	RoleClass string
	Value     string
	Unit      string
	Label     string
}

type chartSeriesView struct {
	RoleClass string // sky | violet | emerald | amber | idle
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
	if c.Size == "full" {
		class = "verso-chart--full"
	}

	view := chartView{Label: c.Label, Class: class, W: dims.W, H: dims.H}
	switch {
	case c.Axis && c.Size == "full":
		// Value gridlines at the top and middle of the range. The labels are HTML
		// overlaid on the plot (not SVG text), so they render at a fixed size instead of
		// scaling with the graph — SVG text in a stretched viewBox looks far bigger than
		// its nominal px.
		scaleY := func(v float64) float64 { return dims.H - dims.Pad - (v/max)*(dims.H-2*dims.Pad) }
		view.GridYs = []float64{scaleY(max), scaleY(max / 2)}
		view.HasAxis = true
		view.AxisTop = fmt.Sprintf("%.0f", max)
		if c.Unit != "" {
			view.AxisTop += " " + c.Unit
		}
		view.AxisMid = fmt.Sprintf("%.0f", max/2)
		view.XStart = c.AxisStart
		view.XEnd = c.AxisEnd
	case dims.Grid:
		view.GridYs = []float64{dims.Pad, (dims.H + dims.Pad) / 2}
	}

	seq := r.chartSeq.Add(1)
	for i, s := range c.Series {
		if len(s.Values) < 2 {
			continue // a line needs at least two points
		}
		sv := chartSeriesView{
			RoleClass: chartRoleClass(s.Role, c.Idle),
			Line:      chartLine(s.Values, dims.W, dims.H, max, dims.Pad),
		}
		if s.Fill {
			sv.GradID = fmt.Sprintf("vc%d-%d", seq, i)
			sv.Area = chartArea(s.Values, dims.W, dims.H, max, dims.Pad)
		}
		if dims.Dot > 0 {
			last := s.Values[len(s.Values)-1]
			sv.DotR = dims.Dot
			sv.DotX = fmt.Sprintf("%.1f", dims.W)
			sv.DotY = fmt.Sprintf("%.1f", dims.H-dims.Pad-(last/max)*(dims.H-2*dims.Pad))
		}
		view.Series = append(view.Series, sv)
	}

	// A full chart may carry a readout: the status headline, its meta line, and the
	// current value of each series (coloured to match its line). Sparklines never do.
	if c.Size == "full" {
		view.Title, view.Live, view.Note = c.Title, c.Live, c.Note
		for _, s := range c.Series {
			if s.Value != "" {
				view.Rates = append(view.Rates, chartRate{
					RoleClass: chartRoleClass(s.Role, c.Idle),
					Value:     s.Value,
					Unit:      c.Unit,
					Label:     s.Label,
				})
			}
		}
		view.Readout = view.Title != "" || view.Note != "" || len(view.Rates) > 0
	}
	return r.execute(out, "chart.html.tmpl", view)
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

// chartLine builds the "M…L…" path through the points: x spreads them evenly across the
// width, y maps each value into the padded height (the top of the scale is max).
func chartLine(vals []float64, w, h, max, pad float64) string {
	var b strings.Builder
	for i, v := range vals {
		x := float64(i) / float64(len(vals)-1) * w
		y := h - pad - (v/max)*(h-2*pad)
		if i == 0 {
			fmt.Fprintf(&b, "M%.1f %.1f", x, y)
		} else {
			fmt.Fprintf(&b, " L%.1f %.1f", x, y)
		}
	}
	return b.String()
}

// chartArea is the same curve closed down to the baseline and back — the filled shape
// under the line.
func chartArea(vals []float64, w, h, max, pad float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "M0 %.1f", h)
	for i, v := range vals {
		x := float64(i) / float64(len(vals)-1) * w
		y := h - pad - (v/max)*(h-2*pad)
		fmt.Fprintf(&b, " L%.1f %.1f", x, y)
	}
	fmt.Fprintf(&b, " L%.1f %.1f Z", w, h)
	return b.String()
}

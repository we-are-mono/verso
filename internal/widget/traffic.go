// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
)

// Traffic is a traffic graph in two lanes, download over upload, each its own
// chart with its own legend, scale and peak, so a slow upload fills its lane
// instead of lying flat under the download, and one time axis under the
// upload: the Overview's Internet graph, and a device's in its drawer
// (ADR-018). Whatever it stands in names it; the graph draws no title. The
// server draws the first paint; the live layer (verso-chart.js) glides both
// lanes as the stream named by Live feeds them. The shell's own composition,
// outside the plugin vocabulary.
type Traffic struct {
	Live  string // the stream that feeds it: "wan", "usage:<mac>"
	Label string // the graph's accessible description
	// ID addresses the graph, so a control outside it (the Traffic card's
	// window) can fetch a window in its place.
	ID string
	// DownNow and UpNow are the newest download and upload, or a past
	// window's averages; Down and Up are the series, oldest first. DownPeak and
	// UpPeak are the window's highest readings where the history keeps them
	// (a past window's per-minute peaks); a lane's peak is never under its
	// series' own highest point. All in Mbit/s, written as every rate is
	// (internal/units).
	DownNow, UpNow   float64
	DownPeak, UpPeak float64
	Down, Up         []float64
	// Place is where the graph stands: an Overview card or a drawer is
	// compact.
	Place ChartPlace
	// Range is the window drawn, one of TrafficRanges; empty is the live
	// minute. A past window is drawn once, its figures its averages. What the
	// graph stands in offers the window (traffic.window).
	Range string
	// Marks are the events over a past window, drawn on both lanes.
	Marks []ChartMark
	// Latency is the gateway's average round trip over a past window, in ms,
	// composed; empty where none was recorded.
	Latency string
}

// TrafficRange is one window a traffic graph can show.
type TrafficRange struct {
	Value, Label string
	Span         time.Duration
}

// TrafficRanges are the windows a graph with history offers, the live minute
// first.
var TrafficRanges = []TrafficRange{
	{"minute", "Last minute", time.Minute},
	{"day", "Last 24 hours", 24 * time.Hour},
	{"week", "Last 7 days", 7 * 24 * time.Hour},
	{"month", "Last 30 days", 30 * 24 * time.Hour},
	{"year", "Last year", 365 * 24 * time.Hour},
}

func (*Traffic) isWidget() {}

func (*Traffic) children() []Widget { return nil }

type trafficView struct {
	Label, Live, ID string
	Seed            string
	Down, Up        template.HTML
	Latency         string
}

func (t *Traffic) renderInto(r *Renderer, out io.Writer, csrf string) error {
	window := TrafficRanges[0]
	for _, candidate := range TrafficRanges {
		if candidate.Value == t.Range {
			window = candidate
		}
	}
	isLive := window == TrafficRanges[0]
	live := t.Live
	if !isLive {
		live = ""
	}
	caption := "on average"
	if isLive {
		caption = "now"
	}
	lane := func(name, role string, now, peak float64, values []float64, axis bool) (template.HTML, error) {
		// A lane's name is a noun, which a catalog keys apart from the
		// button's verb of the same English word.
		name = i18n.Contextual(r.tr, "traffic", name)
		chart := &Chart{
			Label: name, Place: t.Place, Span: window.Span, NoAxis: !axis, Marks: t.Marks,
			Series: []ChartSeries{{
				Label: name, Role: role, Fill: true, Values: values,
				Legend: &ChartLegend{Name: name, Figure: now, Caption: caption, Peak: lanePeak(peak, values)},
			}},
		}
		return renderToHTML(r, chart, csrf)
	}
	down, err := lane("Download", "denim", t.DownNow, t.DownPeak, t.Down, false)
	if err != nil {
		return err
	}
	up, err := lane("Upload", "amethyst", t.UpNow, t.UpPeak, t.Up, true)
	if err != nil {
		return err
	}
	view := trafficView{Label: r.tr(t.Label), Live: live, ID: t.ID, Down: down, Up: up, Latency: t.Latency}
	// Only the live minute seeds the live layer; a past window's gaps are
	// NaN, which JSON cannot carry, and it is never fed.
	if live != "" {
		seed, err := json.Marshal(map[string][]float64{"down": t.Down, "up": t.Up})
		if err != nil {
			return fmt.Errorf("traffic seed: %w", err)
		}
		view.Seed = string(seed)
	}
	return r.execute(out, "traffic.html.tmpl", view)
}

// lanePeak is a lane's peak: the window's highest reading as the history kept
// it, never under the highest point the lane draws.
func lanePeak(kept float64, values []float64) float64 {
	peak := kept
	for _, v := range values {
		if !math.IsNaN(v) {
			peak = max(peak, v)
		}
	}
	return peak
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Traffic is a live traffic graph: its title and what it measures, the newest
// download and upload beside them, and the minute's two series under them on
// the notebook's sand — the overview's Internet graph, and a device's in its
// panel (ADR-018). The server draws the first paint; the live layer
// (verso-chart.js) glides the series as the stream named by Live feeds it.
// The shell's own composition, outside the plugin vocabulary.
type Traffic struct {
	Title string
	Meta  string // what it measures, verbatim: the uplink's device, a device's MAC
	Live  string // the stream that feeds it: "wan", "usage:<mac>"
	Label string // the plot's accessible description
	// DownNow and UpNow are the newest download and upload, already composed;
	// Down and Up are the minute's series in Mbit/s, oldest first.
	DownNow, UpNow string
	Down, Up       []float64
	// Panel stands the graph in a panel beside other facts: a quarter lower,
	// and two cells apart from the blocks above and below it rather than one.
	Panel bool
	// RangeURL, where history stands behind the graph (ADR-019), offers its
	// window as a dropdown on the heading line: choosing one fetches the graph
	// for it from RangeURL with ?range=<value>, in this one's place. Range is
	// the window drawn, one of TrafficRanges; empty is the live minute. A past
	// window is drawn once, its figures its averages.
	RangeURL string
	Range    string
	// Marks are the events over a past window, drawn on its chart.
	Marks []ChartMark
}

// TrafficRange is one window a traffic graph can show.
type TrafficRange struct {
	Value, Label, AxisStart string
}

// TrafficRanges are the windows a graph with history offers, the live minute
// first.
var TrafficRanges = []TrafficRange{
	{"minute", "Last minute", "60 s ago"},
	{"day", "Last 24 hours", "24 h ago"},
	{"week", "Last 7 days", "7 days ago"},
	{"month", "Last 30 days", "30 days ago"},
	{"year", "Last year", "a year ago"},
}

func (*Traffic) isWidget() {}

func (*Traffic) children() []Widget { return nil }

type trafficView struct {
	Title, Meta, Live string
	DownNow, UpNow    string
	Seed              string
	Chart             template.HTML
	Panel             bool
	RangeURL, Range   string
	Ranges            []TrafficRange
}

func (t *Traffic) renderInto(r *Renderer, out io.Writer, csrf string) error {
	window := TrafficRanges[0]
	for _, candidate := range TrafficRanges {
		if candidate.Value == t.Range {
			window = candidate
		}
	}
	live := t.Live
	if window != TrafficRanges[0] {
		live = ""
	}
	chart := &Chart{
		Unit:      "Mbit/s",
		AxisStart: window.AxisStart,
		AxisEnd:   "now",
		Label:     t.Label,
		Short:     t.Panel,
		Marks:     t.Marks,
		Series: []ChartSeries{
			{Label: "down", Role: "emerald", Fill: true, Values: t.Down},
			{Label: "up", Role: "violet", Fill: true, Values: t.Up},
		},
	}
	// Composed here rather than through RenderWithToken, so the chart's captions
	// take the localization walk here (ADR-012).
	r.translate(chart)
	chartHTML, err := renderToHTML(r, chart, csrf)
	if err != nil {
		return err
	}
	view := trafficView{
		Title: r.tr(t.Title), Meta: t.Meta, Live: live,
		DownNow: t.DownNow, UpNow: t.UpNow, Chart: chartHTML, Panel: t.Panel,
		RangeURL: t.RangeURL, Range: window.Value,
	}
	if t.RangeURL != "" {
		for _, option := range TrafficRanges {
			view.Ranges = append(view.Ranges, TrafficRange{Value: option.Value, Label: r.tr(option.Label)})
		}
	}
	// Only the live minute seeds the live layer; a past window's gaps are
	// NaN, which JSON cannot carry, and it is never fed.
	if live != "" {
		seed, err := json.Marshal(map[string][]float64{"down": t.Down, "up": t.Up})
		if err != nil {
			return err
		}
		view.Seed = string(seed)
	}
	return r.execute(out, "traffic.html.tmpl", view)
}

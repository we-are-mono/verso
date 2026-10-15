// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"time"
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
	// DownNow and UpNow are the newest download and upload, or a past
	// window's averages; Down and Up are the series, oldest first. All in
	// Mbit/s, written as every rate is (internal/units).
	DownNow, UpNow float64
	Down, Up       []float64
	// Panel stands the graph in a panel beside other facts: a quarter lower,
	// and two cells apart from the blocks above and below it rather than one.
	Panel bool
	// Card stands the graph in an Overview card (ADR-022), whose header already
	// names it: no title, and no sand band or rule of its own.
	Card bool
	// RangeURL, where history stands behind the graph (ADR-019), offers its
	// window as a dropdown on the heading line: choosing one fetches the graph
	// for it from RangeURL with ?range=<value>, in this one's place. Range is
	// the window drawn, one of TrafficRanges; empty is the live minute. A past
	// window is drawn once, its figures its averages.
	RangeURL string
	Range    string
	// Marks are the events over a past window, drawn on its chart.
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
	Title, Meta, Live string
	// DownNow and UpNow are the figures, and DownLabel and UpLabel their
	// unit and direction after them ("Mb/s down").
	DownNow, UpNow     string
	DownLabel, UpLabel string
	Seed               string
	Chart              template.HTML
	Panel, Card        bool
	RangeURL, Range    string
	Ranges             []TrafficRange
	Latency            string
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
	place := PlacePage
	if t.Card || t.Panel {
		place = PlaceCompact
	}
	chart := &Chart{
		Span:  window.Span,
		Live:  window == TrafficRanges[0],
		Label: t.Label,
		Place: place,
		Marks: t.Marks,
		Series: []ChartSeries{
			{Label: "down", Role: "emerald", Fill: true, Values: t.Down},
			{Label: "up", Role: "violet", Fill: true, Values: t.Up},
		},
	}
	// In an Overview card the download is denim and the upload amethyst, each
	// over a soft wash of its own.
	if t.Card {
		chart.Series[0].Role, chart.Series[1].Role = "denim", "amethyst"
	}
	// Composed here rather than through RenderWithToken, so the chart's captions
	// take the localization walk here (ADR-012).
	r.translate(chart)
	chartHTML, err := renderToHTML(r, chart, csrf)
	if err != nil {
		return err
	}
	down, up := r.numbers().Rate(t.DownNow*1e6), r.numbers().Rate(t.UpNow*1e6)
	downLabel, upLabel := "%s down", "%s up"
	if live == "" {
		downLabel, upLabel = "%s down on average", "%s up on average"
	}
	view := trafficView{
		Title: r.tr(t.Title), Meta: t.Meta, Live: live,
		DownNow: down.Value, UpNow: up.Value,
		DownLabel: fmt.Sprintf(r.tr(downLabel), down.Unit), UpLabel: fmt.Sprintf(r.tr(upLabel), up.Unit),
		Chart: chartHTML, Panel: t.Panel, Card: t.Card,
		RangeURL: t.RangeURL, Range: window.Value, Latency: t.Latency,
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

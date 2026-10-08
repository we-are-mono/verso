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
}

func (*Traffic) isWidget() {}

func (*Traffic) children() []Widget { return nil }

type trafficView struct {
	Title, Meta, Live string
	DownNow, UpNow    string
	Seed              string
	Chart             template.HTML
}

func (t *Traffic) renderInto(r *Renderer, out io.Writer, csrf string) error {
	chart := &Chart{
		Unit:      "Mbit/s",
		AxisStart: "60 s ago",
		AxisEnd:   "now",
		Label:     t.Label,
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
	seed, err := json.Marshal(map[string][]float64{"down": t.Down, "up": t.Up})
	if err != nil {
		return err
	}
	return r.execute(out, "traffic.html.tmpl", trafficView{
		Title: r.tr(t.Title), Meta: t.Meta, Live: t.Live,
		DownNow: t.DownNow, UpNow: t.UpNow, Seed: string(seed), Chart: chartHTML,
	})
}

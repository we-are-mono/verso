// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderTraffic: a live traffic graph is the overview's Internet graph
// wherever it stands — its title and what it measures, the newest download and
// upload beside them, and the minute's two series under them — and it names
// the stream that feeds it, with the series it opens on as the live layer's
// seed.
func TestRenderTraffic(t *testing.T) {
	got := render(t, newRenderer(t), &Traffic{
		Title: "Traffic", Meta: "42:e6:ad:ff:b7:af", Live: "usage:42:e6:ad:ff:b7:af",
		Label:   "Device traffic — download and upload, last minute",
		DownNow: 38.2, UpNow: 1.1,
		Down: []float64{0, 10, 38.2}, Up: []float64{0, 0.5, 1.1},
	})
	for _, want := range []string{
		`aria-label="Traffic"`, ">Traffic</h2>", ">42:e6:ad:ff:b7:af<",
		`data-verso-live-chart="usage:42:e6:ad:ff:b7:af"`,
		`data-verso-live-seed="{&#34;down&#34;:[0,10,38.2],&#34;up&#34;:[0,0.5,1.1]}"`,
		"data-verso-live-down>38<", "data-verso-live-up>1.1<",
		"data-verso-live-down-label>Mb/s down<", "data-verso-live-up-label>Mb/s up<",
		`<svg class="verso-chart`, "verso-chart--emerald", "verso-chart--violet",
		"60 s ago", ">now<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("traffic graph missing %q", want)
		}
	}
	if !strings.Contains(got, `data-chart-place="page"`) {
		t.Error("a graph on a page is ruled in a page's 32px squares")
	}

	// In a panel it is ruled in 24px squares; and 40px apart from what stands
	// above and below it, its top border still on a line.
	panel := render(t, newRenderer(t), &Traffic{Title: "Usage", Live: "usage:x", Panel: true, Down: []float64{0, 1}, Up: []float64{0, 1}})
	if !strings.Contains(panel, `data-chart-place="compact"`) {
		t.Error("a graph in a panel is ruled in 24px squares")
	}
	if !strings.Contains(panel, "mt-9.75 mb-10") || strings.Contains(panel, "-mt-px") {
		t.Error("a graph in a panel stands two cells from its neighbours")
	}
	if strings.Contains(panel, "<select") {
		t.Error("a graph with no history to read offers no window")
	}
}

// TestTrafficOffersItsWindow: a graph with history behind it offers its window
// as a dropdown on its heading line, which fetches the graph for the window
// chosen in its place. The minute is live; a past window is drawn once, not
// fed by the stream, and its figures are its averages, said as such.
func TestTrafficOffersItsWindow(t *testing.T) {
	live := render(t, newRenderer(t), &Traffic{
		Title: "Internet traffic", Live: "wan", RangeURL: "/overview/traffic",
		Down: []float64{0, 1}, Up: []float64{0, 1},
	})
	for _, want := range []string{
		`<select name="range"`, `hx-get="/overview/traffic"`, `hx-trigger="change"`,
		`hx-target="closest section"`, `hx-swap="outerHTML"`,
		`<option value="minute" selected>Last minute</option>`,
		`<option value="day">Last 24 hours</option>`, `<option value="week">Last 7 days</option>`,
		`<option value="month">Last 30 days</option>`, `<option value="year">Last year</option>`,
		`data-verso-live-chart="wan"`, "60 s ago",
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live graph missing %q", want)
		}
	}

	day := render(t, newRenderer(t), &Traffic{
		Title: "Internet traffic", Live: "wan", RangeURL: "/overview/traffic", Range: "day",
		DownNow: 12.3, UpNow: 1234, Down: []float64{0, 1}, Up: []float64{0, 1},
	})
	for _, want := range []string{
		`<option value="day" selected>Last 24 hours</option>`, ">24 h<",
		">12<", "Mb/s down on average", ">1.2<", "Gb/s up on average",
	} {
		if !strings.Contains(day, want) {
			t.Errorf("a day's graph missing %q", want)
		}
	}
	for _, unwanted := range []string{"data-verso-live-chart", "data-verso-live-seed", "data-verso-live-down", "60 s ago"} {
		if strings.Contains(day, unwanted) {
			t.Errorf("a day's graph carries %q, but it is not live", unwanted)
		}
	}
	if strings.Contains(day, "ms to gateway") {
		t.Error("a window with no latency recorded states one")
	}

	// The gateway's latency over the window stands beside the traffic, as a
	// figure of its own.
	probed := render(t, newRenderer(t), &Traffic{
		Title: "Internet traffic", Live: "wan", RangeURL: "/overview/traffic", Range: "day",
		Latency: "12", Down: []float64{0, 1}, Up: []float64{0, 1},
	})
	if !strings.Contains(probed, ">12<") || !strings.Contains(probed, "ms to gateway on average") {
		t.Error("a probed window does not state the gateway's latency")
	}
}

// TestACardsSeriesBothWash: in an Overview card each series stands over its
// own soft wash, the download's denim and the upload's amethyst alike.
func TestACardsSeriesBothWash(t *testing.T) {
	got := render(t, newRenderer(t), &Traffic{Title: "Internet traffic", Card: true, Down: []float64{0, 1}, Up: []float64{0, 1}})
	for _, role := range []string{"denim", "amethyst"} {
		series := got[strings.Index(got, "verso-chart--"+role):]
		series = series[:strings.Index(series, "</g>")]
		if !strings.Contains(series, "verso-chart-area") {
			t.Errorf("the %s series has no wash: %s", role, series)
		}
	}
}

// TestACardsWindowRidesTheLegend: in an Overview card the window is the compact
// dropdown on the figures' first line at its right end, and swaps the graph it
// heads, so the graph
// that comes back carries it with the chosen window selected. The card's
// header carries no control.
func TestACardsWindowRidesTheLegend(t *testing.T) {
	got := render(t, newRenderer(t), &Traffic{
		Title: "Internet traffic", Card: true, RangeURL: "/overview/traffic", Range: "week",
		Down: []float64{0, 1}, Up: []float64{0, 1},
	})
	legend := got[strings.Index(got, "verso-traffic-card"):]
	legend = legend[:strings.Index(legend, "data-verso-chart")]
	for _, want := range []string{
		"Mb/s down", `<select name="range"`, `hx-target="closest section"`, `hx-swap="outerHTML"`,
		`<option value="week" selected>Last 7 days</option>`, "flex-1", `class="h-6 cursor-pointer`,
	} {
		if !strings.Contains(legend, want) {
			t.Errorf("the card's legend line missing %q:\n%s", want, legend)
		}
	}
	if strings.Index(legend, "Mb/s up") > strings.Index(legend, "<select") {
		t.Error("the window follows the figures on their line")
	}
	overview := render(t, newRenderer(t), &Overview{TrafficRangeURL: "/overview/traffic"})
	if strings.Contains(overview, "data-overview-card-control") || strings.Count(overview, `name="range"`) != 1 {
		t.Error("the window stands once, in the graph, not in the card's header")
	}
}

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
		DownNow: "38.2", UpNow: "1.1",
		Down: []float64{0, 10, 38.2}, Up: []float64{0, 0.5, 1.1},
	})
	for _, want := range []string{
		`aria-label="Traffic"`, ">Traffic</h2>", ">42:e6:ad:ff:b7:af<",
		`data-verso-live-chart="usage:42:e6:ad:ff:b7:af"`,
		`data-verso-live-seed="{&#34;down&#34;:[0,10,38.2],&#34;up&#34;:[0,0.5,1.1]}"`,
		"data-verso-live-down>38.2<", "data-verso-live-up>1.1<",
		"Mbit/s down", "Mbit/s up",
		`<svg class="verso-chart`, "verso-chart--emerald", "verso-chart--violet",
		"60 s ago", ">now<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("traffic graph missing %q", want)
		}
	}
	if !strings.Contains(got, "h-60") {
		t.Error("the graph stands at the overview's 240px by default")
	}

	// In a panel it stands a quarter shorter, 180px: nine cells, still on the
	// grid; and two cells apart from what stands above and below it, where the
	// panel's blocks stand one apart, its top border still on a line.
	panel := render(t, newRenderer(t), &Traffic{Title: "Usage", Live: "usage:x", Panel: true, Down: []float64{0, 1}, Up: []float64{0, 1}})
	if !strings.Contains(panel, "h-45") || strings.Contains(panel, "h-60") {
		t.Error("a graph in a panel stands at 180px")
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
		DownNow: "12.3", UpNow: "0.4", Down: []float64{0, 1}, Up: []float64{0, 1},
	})
	for _, want := range []string{
		`<option value="day" selected>Last 24 hours</option>`, "24 h ago",
		">12.3<", "Mbit/s down on average", "Mbit/s up on average",
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
}

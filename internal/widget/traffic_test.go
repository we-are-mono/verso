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
}

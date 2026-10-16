// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// trafficLanes splits a traffic graph into its download and upload lanes.
func trafficLanes(t *testing.T, html string) (down, up string) {
	t.Helper()
	before, after, ok := strings.Cut(html, `data-verso-live-lane="up"`)
	if !ok || !strings.Contains(before, `data-verso-live-lane="down"`) {
		t.Fatalf("the graph is not two lanes, download over upload:\n%s", html)
	}
	return before, after
}

// TestTrafficStandsInTwoLanes: download over upload, each lane a chart with its
// own legend in its own hue, its series washed, and one time axis under the
// upload; the graph names the stream that feeds it and seeds the live layer,
// and draws no title of its own.
func TestTrafficStandsInTwoLanes(t *testing.T) {
	got := render(t, newRenderer(t), &Traffic{
		Live: "usage:42:e6:ad:ff:b7:af", Label: "Device traffic — download and upload, last minute",
		DownNow: 38.2, UpNow: 1.1, Place: PlaceCompact,
		Down: []float64{0, 10, 38.2}, Up: []float64{0, 0.5, 1.1},
	})
	for _, want := range []string{
		`data-verso-live-chart="usage:42:e6:ad:ff:b7:af"`,
		`data-verso-live-seed="{&#34;down&#34;:[0,10,38.2],&#34;up&#34;:[0,0.5,1.1]}"`,
		`aria-label="Device traffic — download and upload, last minute"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("traffic graph missing %q", want)
		}
	}
	if strings.Contains(got, "<h2") {
		t.Error("the graph draws a title of its own")
	}
	down, up := trafficLanes(t, got)
	for lane, wants := range map[string][]string{
		down: {"verso-chart--denim", "bg-denim", ">Download<", ">38\U0000202fMb/s<", ">now<", `data-chart-place="compact"`},
		up:   {"verso-chart--amethyst", "bg-amethyst", ">Upload<", ">1.1\U0000202fMb/s<", ">now<", "data-verso-chart-window", ">60\U0000202fs<", ">15\U0000202fs<"},
	} {
		for _, want := range wants {
			if !strings.Contains(lane, want) {
				t.Errorf("lane missing %q:\n%s", want, lane)
			}
		}
		if !strings.Contains(lane, "verso-chart-area") {
			t.Errorf("a lane's series has no wash:\n%s", lane)
		}
	}
	if strings.Contains(down, "data-verso-chart-window") {
		t.Error("the download lane draws a time axis; the upload's serves both")
	}
}

// TestEachLaneHasItsOwnScale: a lopsided line fills both lanes, each on the
// round scale over its own tallest point.
func TestEachLaneHasItsOwnScale(t *testing.T) {
	down, up := trafficLanes(t, render(t, newRenderer(t), &Traffic{
		Down: []float64{0, 57}, Up: []float64{0, 8.5},
	}))
	if !strings.Contains(down, `style="top:0.0%">80<`) {
		t.Errorf("the download lane's scale is not its own:\n%s", down)
	}
	if !strings.Contains(up, `style="top:0.0%">10<`) {
		t.Errorf("the upload lane's scale is not its own:\n%s", up)
	}
}

// TestAPastWindowSaysItsAveragesAndPeaks: a past window's figures are its
// averages, said as such, and each lane's peak is the highest the history
// kept, never under the highest point the lane draws.
func TestAPastWindowSaysItsAveragesAndPeaks(t *testing.T) {
	down, up := trafficLanes(t, render(t, newRenderer(t), &Traffic{
		Range: "day", DownNow: 12.3, UpNow: 1234, DownPeak: 57,
		Down: []float64{0, 20}, Up: []float64{0, 1500},
	}))
	for lane, wants := range map[string][]string{
		down: {">12\U0000202fMb/s<", ">on average<", ">peak<", ">57\U0000202fMb/s<"},
		up:   {">1.2\U0000202fGb/s<", ">on average<", ">1.5\U0000202fGb/s<", ">24\U0000202fh<", ">now<"},
	} {
		for _, want := range wants {
			if !strings.Contains(lane, want) {
				t.Errorf("lane missing %q:\n%s", want, lane)
			}
		}
	}
}

// TestAPastWindowIsNotLive: the minute is live; a past window is drawn once,
// not fed by the stream. The graph carries no window choice of its own: what
// it stands in offers one.
func TestAPastWindowIsNotLive(t *testing.T) {
	live := render(t, newRenderer(t), &Traffic{Live: "wan", ID: "g", Down: []float64{0, 1}, Up: []float64{0, 1}})
	if !strings.Contains(live, `data-verso-live-chart="wan"`) || !strings.Contains(live, `id="g"`) {
		t.Errorf("the live graph is not fed or not addressable:\n%s", live)
	}
	day := render(t, newRenderer(t), &Traffic{Live: "wan", Range: "day", Down: []float64{0, 1}, Up: []float64{0, 1}})
	for _, unwanted := range []string{"data-verso-live-chart", "data-verso-live-seed", ">60\U0000202fs<", "<select"} {
		if strings.Contains(day, unwanted) {
			t.Errorf("a day's graph carries %q", unwanted)
		}
	}
}

// TestAWindowStatesTheGatewaysLatency: the gateway's latency over a past
// window stands under the lanes as a fact of its own, and only when recorded.
func TestAWindowStatesTheGatewaysLatency(t *testing.T) {
	probed := render(t, newRenderer(t), &Traffic{Range: "day", Latency: "12", Down: []float64{0, 1}, Up: []float64{0, 1}})
	for _, want := range []string{">Gateway<", ">12&#x202F;ms<", ">on average<"} {
		if !strings.Contains(probed, want) {
			t.Errorf("a probed window missing %q", want)
		}
	}
	if quiet := render(t, newRenderer(t), &Traffic{Range: "day", Down: []float64{0, 1}, Up: []float64{0, 1}}); strings.Contains(quiet, "Gateway") {
		t.Error("a window with no latency recorded states one")
	}
}

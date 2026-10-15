// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/sensors"
)

// temperatureServer is a DK whose CPU recorded two minutes this hour: 48.2 and
// 49.2 °C on average, the second touching 59.0 °C at its peak.
func temperatureServer(t *testing.T) *Server {
	t.Helper()
	s := newServer(t, fakeBackend{})
	s.inventory = func(*sensors.Profile) sensors.Inventory { return dkInventory() }
	store, err := history.Open(history.Memory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	now := time.Now().Truncate(time.Minute)
	store.Add(
		history.Point{Series: "temp.cluster-thermal", Time: now.Add(-10 * time.Minute), Average: 48.2, Peak: 49},
		history.Point{Series: "temp.cluster-thermal", Time: now.Add(-9 * time.Minute), Average: 49.2, Peak: 59},
	)
	return s
}

// TestTheTemperatureGraphAnswersAChosenReading: the reading and window chosen
// in the field come back as the graph alone, under the id the field targets:
// each period's average, the window's average as its figure, its highest
// reading as its peak, all in °C.
func TestTheTemperatureGraphAnswersAChosenReading(t *testing.T) {
	status, body := getSignedIn(t, temperatureServer(t), temperaturePath+"?sensor=temp.cluster-thermal&range=day")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", status, body)
	}
	for _, want := range []string{
		`id="hardware-temperature-graph"`, `data-chart-place="page"`, ">cluster<",
		">48.7 °C<", ">on average<", ">peak<", ">59.0 °C<", `style="top:0.0%">80<`, "data-verso-chart-unit>°C<", ">24 h<", `class="verso-chart-line`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the graph missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<select") || strings.Contains(body, "<html") {
		t.Error("the answer is more than the graph")
	}
}

// TestTheTemperatureGraphReadsOnlyWhatItOffers: a reading the board does not
// offer (a faulted channel, any other series) and a window the field does not
// offer are refused, so the route never reads an arbitrary series.
func TestTheTemperatureGraphReadsOnlyWhatItOffers(t *testing.T) {
	s := temperatureServer(t)
	for _, query := range []string{
		"?sensor=temp.temp2&range=day",          // faulted, so no choice
		"?sensor=iface.wan0.rx&range=day",       // not a temperature at all
		"?sensor=temp.cluster-thermal&range=1h", // not a window offered
		"?sensor=temp.cluster-thermal&range=minute",
		"",
	} {
		if status, _ := getSignedIn(t, s, temperaturePath+query); status != http.StatusBadRequest {
			t.Errorf("%q answered %d, want 400", query, status)
		}
	}
}

// TestATemperatureWithNoHistorySaysSo: a reading never recorded, or a router
// that keeps no history, says so where the plot would stand.
func TestATemperatureWithNoHistorySaysSo(t *testing.T) {
	s := temperatureServer(t)
	_, body := getSignedIn(t, s, temperaturePath+"?sensor="+strings.ReplaceAll("temp.temperature.sfp · xfi1", " ", "%20")+"&range=week")
	if !strings.Contains(body, "Nothing was recorded for SFP&#43; module xfi1 over this window.") || strings.Contains(body, "<svg") {
		t.Errorf("a module never recorded draws a plot or says nothing:\n%s", body)
	}
	s.historyStore = nil
	_, body = getSignedIn(t, s, temperaturePath+"?sensor=temp.cluster-thermal&range=day")
	if !strings.Contains(body, "This router keeps no history.") {
		t.Errorf("a router with no history does not say so:\n%s", body)
	}
}

// TestTheHardwarePageOpensOnTheCPUsDay: the page's first paint already holds
// the CPU's last 24 hours, so the graph needs no second request.
func TestTheHardwarePageOpensOnTheCPUsDay(t *testing.T) {
	status, body := getSignedIn(t, temperatureServer(t), "/system/hardware")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	graph := body[strings.Index(body, `id="hardware-temperature-graph"`)+1:]
	for _, want := range []string{">48.7 °C<", ">59.0 °C<", `class="verso-chart-line`} {
		if !strings.Contains(graph, want) {
			t.Errorf("the first paint's graph missing %q", want)
		}
	}
	if !strings.Contains(body, `<option value="temp.cluster-thermal" selected>`) || !strings.Contains(body, `<option value="day" selected>`) {
		t.Error("the field does not open on the CPU's day")
	}
}

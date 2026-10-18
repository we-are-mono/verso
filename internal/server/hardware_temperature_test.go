// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
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
	s := newServer(t, fakeBackend{access: true})
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
		`id="hardware-temperature-graph"`, `data-verso-place="page"`, ">cluster<",
		">48.7\U0000202f°C<", ">on average<", ">peak<", ">59.0\U0000202f°C<", `style="top:0.0%">80<`, "data-verso-chart-unit>°C<", ">24\U0000202fh<", `class="verso-chart__line"`,
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
	for _, want := range []string{">48.7\U0000202f°C<", ">59.0\U0000202f°C<", `class="verso-chart__line"`} {
		if !strings.Contains(graph, want) {
			t.Errorf("the first paint's graph missing %q", want)
		}
	}
	if !strings.Contains(body, `<option value="temp.cluster-thermal" selected>`) || !strings.Contains(body, `<option value="day" selected>`) {
		t.Error("the field does not open on the CPU's day")
	}
}

// TestTheTemperatureHistoryNeedsTheSystemGrant: history.db's temperatures are
// the shell's own store, so a session rpcd refuses the system info read, or
// does not answer for, is drawn no graph and refused one, while the table of
// what the sensors read now stays; the system grant is the one the events
// page and the graphs mark the router starting under.
func TestTheTemperatureHistoryNeedsTheSystemGrant(t *testing.T) {
	for _, c := range []struct {
		name string
		b    fakeBackend
	}{
		{"refused", fakeBackend{grant: grantAllBut("ubus system info")}},
		{"unanswered", fakeBackend{accessErr: errors.New("rpcd gone")}},
	} {
		s := temperatureServer(t)
		s.backend = c.b
		if status, body := getSignedIn(t, s, temperaturePath+"?sensor=temp.cluster-thermal&range=day"); status != http.StatusForbidden || strings.Contains(body, "°C") {
			t.Errorf("%s: the graph answered %d:\n%s", c.name, status, body)
		}
		_, body := getSignedIn(t, s, "/system/hardware")
		for _, never := range []string{`id="hardware-temperature-graph"`, ">Show chart for<", ">48.7\U0000202f°C<"} {
			if strings.Contains(body, never) {
				t.Errorf("%s: the page draws %q", c.name, never)
			}
		}
		if !strings.Contains(body, ">cluster-thermal<") {
			t.Errorf("%s: the page lists no reading", c.name)
		}
	}
}

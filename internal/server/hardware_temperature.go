// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/units"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// The Temperatures card charts one reading's history over a window (ADR-019),
// chosen in the field over its graph: "Show chart for [CPU] over [Last 24
// hours]".

// temperaturePath answers the Temperatures graph for ?sensor=<series>&range=.
const temperaturePath = "/system/hardware/temperature"

// temperatureGraphID addresses the graph, so the field fetches another
// reading or window in its place.
const temperatureGraphID = "hardware-temperature-graph"

// temperatureChoice is one reading the graph can show: the series its history
// is under, and its name as the page shows it.
type temperatureChoice struct {
	Series, Name string
}

// temperatureChoices are the readings the graph offers: the table's rows in
// order, then each SFP module's own temperature ("SFP+ module xfi0"). A
// faulted channel records nothing, so it is no choice.
func temperatureChoices(tr func(string) string, inv sensors.Inventory) []temperatureChoice {
	var out []temperatureChoice
	for _, t := range inv.PageTemps() {
		if !t.Fault && t.Series != "" {
			out = append(out, temperatureChoice{t.Series, t.Name})
		}
	}
	for _, f := range inv.Fibers {
		for _, t := range inv.Temps {
			if t.Fiber && t.Source == f.Source && !t.Fault && t.Series != "" {
				out = append(out, temperatureChoice{t.Series, tr("SFP+ module") + " " + f.Cage})
			}
		}
	}
	return out
}

// temperatureChoiceOf is the offered reading recorded under series; only an
// offered one is ever read.
func temperatureChoiceOf(choices []temperatureChoice, series string) (temperatureChoice, bool) {
	for _, c := range choices {
		if c.Series == series {
			return c, true
		}
	}
	return temperatureChoice{}, false
}

// temperatureField is the field over the graph: the reading, then the window,
// read as one sentence; each choice fetches the graph for both into its place
// and stages nothing.
func temperatureField(choices []temperatureChoice, series, window string) widget.Widget {
	fetch := &widget.FieldFetch{URL: temperaturePath, Target: "#" + temperatureGraphID}
	readings := make([]widget.Option, 0, len(choices))
	for _, c := range choices {
		readings = append(readings, widget.Option{Value: c.Series, Label: c.Name})
	}
	windows := make([]widget.Option, 0, len(historyWindows))
	for _, w := range historyWindows {
		windows = append(windows, widget.Option{Value: w.Value, Label: w.Label})
	}
	return &widget.Grid{Style: "form", Label: "Show chart for", Join: "over", Children: []widget.Widget{
		&widget.Field{Name: "sensor", Label: "Reading", Kind: "select", Value: series, Options: readings, OptionsVerbatim: true, Fetch: fetch},
		&widget.Field{Name: "range", Label: "Window", Kind: "select", Value: window, Options: windows, Fetch: fetch},
	}}
}

// temperatureGraph is a reading's history over a past window: each period's
// average, the window's average as its figure and its highest reading as its
// peak, a reboot as its mark; or, where nothing was recorded or the router
// keeps no history, a sentence saying so.
func (s *Server) temperatureGraph(ctx context.Context, tr func(string) string, c temperatureChoice, w historyWindow) *widget.Chart {
	chart := &widget.Chart{
		ID: temperatureGraphID, Unit: units.UnitCelsius, Place: widget.PlacePage, Span: w.Span(),
		Label:    fmt.Sprintf(tr("%s temperature, %s"), c.Name, strings.ToLower(tr(w.Label))),
		Verbatim: true,
		Empty:    fmt.Sprintf(tr("Nothing was recorded for %s over this window."), c.Name),
	}
	if s.historyStore == nil {
		chart.Empty = tr("This router keeps no history.")
		return chart
	}
	now := time.Now()
	start, end := w.bounds(now)
	points, err := s.historyStore.Points(ctx, c.Series, w.Res, start, end)
	if err != nil {
		log.Printf("verso: hardware: %s history unavailable: %v", c.Series, err)
		return chart
	}
	peak := 0.0
	for _, p := range points {
		peak = max(peak, p.Peak)
	}
	values := history.Slots(points, func(p history.Point) float64 { return p.Average }, start, w.Res, w.N)
	if events, err := s.historyStore.Events(ctx, start, end); err == nil {
		for _, m := range eventMarks(events, start, end, now, tr) {
			if m.Kind == "boot" {
				chart.Marks = append(chart.Marks, m)
			}
		}
	}
	chart.Series = []widget.ChartSeries{{
		Label: c.Name, Role: "denim", Fill: true, Values: values,
		Legend: &widget.ChartLegend{Name: c.Name, Figure: mean(values), Caption: "on average", Peak: peak},
	}}
	return chart
}

// sensorInventory is what the board's sensors read now, resolved by its
// profile.
func (s *Server) sensorInventory(p *sensors.Profile) sensors.Inventory {
	if s.inventory != nil {
		return s.inventory(p)
	}
	return sensors.ResolveAll(p, &sensors.Reader{})
}

// mayReadTemperatures reports whether rpcd grants sid the read history.db's
// temperatures reach a session under (ADR-007): no rpcd read answers a
// sensor, so it is the system info read, the router's own state, which the
// router starting is shown under on the events page and on every graph.
func (s *Server) mayReadTemperatures(ctx context.Context, sid string) bool {
	return s.grantsIn(ctx, sid).may(ctx, systemGrant)
}

// handleTemperatureHistory answers the Temperatures graph for the reading and
// window chosen in its field, in its place. Only a session that may read the
// history is answered, and only a reading the board offers and a window the
// field offers are read.
func (s *Server) handleTemperatureHistory(w http.ResponseWriter, r *http.Request) {
	if !s.mayReadTemperatures(r.Context(), s.sessionSID(r)) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	board, err := s.backend.Board(r.Context(), s.sessionSID(r))
	if err != nil {
		log.Printf("verso: hardware: board unavailable: %v", err)
	}
	profile, _ := sensors.LoadProfile(profiles.FS, board.BoardName)
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	choice, offered := temperatureChoiceOf(temperatureChoices(tr, s.sensorInventory(profile)), r.URL.Query().Get("sensor"))
	window, known := historyWindowOf(r.URL.Query().Get("range"))
	if !offered || !known {
		http.Error(w, "no such reading or window", http.StatusBadRequest)
		return
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, s.temperatureGraph(r.Context(), tr, choice, window), s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body.String()))
}

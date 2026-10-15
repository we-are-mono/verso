// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/units"
	"github.com/we-are-mono/verso/internal/widget"
)

const (
	eventsPath = "/system/events"
	// stoppedMark is left in the history directory when the shell stops
	// cleanly, so the next boot can tell a reboot from a power cut.
	stoppedMark = "stopped"
	// wanWatchEvery is how often the kernel's default routes are read.
	wanWatchEvery = 10 * time.Second
)

// wanWatch turns readings of the uplink into the moments it changed.
type wanWatch struct {
	known  bool
	up     bool
	device string
	since  time.Time
}

// observe records nothing for the first reading, which only sets the state.
// The internet is up while a default route stands in the main table: one in
// a policy table carries only what is marked for it (a VPN's table).
func (w *wanWatch) observe(now time.Time, state openwrt.WANState) []history.Event {
	var main openwrt.WANState
	for _, device := range state.Devices {
		if slices.ContainsFunc(device.Routes, func(r openwrt.WANRoute) bool { return r.Main }) {
			main.Devices = append(main.Devices, device)
		}
	}
	device := ""
	if primary, ok := main.Primary(); ok {
		device = primary.Device
	}
	up := main.Up()
	defer func() {
		if !w.known || up != w.up {
			w.since = now
		}
		w.known, w.up = true, up
		if up {
			w.device = device
		}
	}()
	if !w.known || up == w.up {
		return nil
	}
	if !up {
		return []history.Event{{Time: now, Kind: "wan.down", Subject: w.device}}
	}
	after, _ := json.Marshal(map[string]int64{"after": int64(now.Sub(w.since) / time.Second)})
	return []history.Event{{Time: now, Kind: "wan.up", Subject: device, Detail: string(after)}}
}

type bootDetail struct {
	Clean bool   `json:"clean"`
	ID    string `json:"id"`
}

// bootEvent is this boot's event unless the last one recorded is this boot's
// already (a shell restarted within it). Clean says the shell was stopped
// before the router went down.
func bootEvent(id string, booted time.Time, last history.Event, clean bool) (history.Event, bool) {
	var before bootDetail
	_ = json.Unmarshal([]byte(last.Detail), &before)
	if before.ID == id {
		return history.Event{}, false
	}
	detail, _ := json.Marshal(bootDetail{Clean: clean, ID: id})
	return history.Event{Time: booted, Kind: "boot", Detail: string(detail)}, true
}

// bootTime is the kernel's boot time from /proc/stat's btime line.
func bootTime(stat []byte) (time.Time, error) {
	for line := range bytes.Lines(stat) {
		if rest, ok := bytes.CutPrefix(line, []byte("btime ")); ok {
			seconds, err := strconv.ParseInt(string(bytes.TrimSpace(rest)), 10, 64)
			return time.Unix(seconds, 0), err
		}
	}
	return time.Time{}, fmt.Errorf("stat: no btime")
}

// recordBoot records this boot once, and takes the clean-stop mark.
func recordBoot(ctx context.Context, store *history.Store, dir string) {
	id, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return
	}
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	booted, err := bootTime(stat)
	if err != nil {
		return
	}
	mark := filepath.Join(dir, stoppedMark)
	_, statErr := os.Stat(mark)
	last, _, err := store.Latest(ctx, "boot")
	if err != nil {
		return
	}
	if e, ok := bootEvent(strings.TrimSpace(string(id)), booted, last, statErr == nil); ok {
		if err := store.Record(ctx, e); err != nil {
			log.Printf("verso: boot not recorded: %v", err)
		}
	}
	_ = os.Remove(mark)
}

// markStopped leaves the clean-stop mark beside the history.
func markStopped(dir string) {
	_ = os.WriteFile(filepath.Join(dir, stoppedMark), nil, 0o644)
}

// watchWAN records the uplink going down and coming back until ctx ends.
func watchWAN(ctx context.Context, store *history.Store, read func() (openwrt.WANState, error)) {
	var w wanWatch
	tick := time.NewTicker(wanWatchEvery)
	defer tick.Stop()
	for {
		if state, err := read(); err == nil {
			for _, e := range w.observe(time.Now(), state) {
				if err := store.Record(ctx, e); err != nil {
					log.Printf("verso: event not recorded: %v", err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// handleEvents lists what happened, newest first (ADR-019), as far as the
// session may see it: each event under the grant its reading needs, an
// apply under its entry in config history (ADR-007).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	now := time.Now()
	events, err := s.historyStore.Events(r.Context(), time.Unix(0, 0), now.Add(time.Minute))
	if err != nil {
		http.Error(w, "history unavailable", http.StatusInternalServerError)
		return
	}
	entries, err := s.historyStore.Changes(r.Context(), changesListed)
	if err != nil {
		http.Error(w, "history unavailable", http.StatusInternalServerError)
		return
	}
	g := s.grantsFor(s.sessionSID(r))
	view := newEventsView(r.Context(), g, entries, s.readsAllHistory(r.Context(), g, entries))
	table := &widget.Table{
		Columns: []widget.TableColumn{
			{Label: "When", Width: widget.MeasureName},
			{Label: "Interface", Kind: "mono", Width: widget.MeasureName},
			{Label: "Event", Kind: "status"},
			{Kind: "actions", Width: widget.MeasureShort},
		},
		EmptyText: "Nothing has happened yet. The router records when the internet goes down and comes back, and when it starts.",
	}
	for _, e := range events {
		if shown, ok := view.show(r.Context(), e); ok {
			table.Rows = append(table.Rows, eventRow(shown, now, tr))
		}
	}
	page := &widget.Stack{Children: append(uncheckedNotice(g, table), table)}
	// While the history is not being saved, the page that reads it says so,
	// and why (ADR-019).
	if h := s.historyStore.Health(); !h.OK {
		page.Children = append([]widget.Widget{&widget.Callout{Variant: "warning", Compact: true, Verbatim: h.Cause,
			Body: "History is not being saved. Measurements and events are kept in memory until the storage answers again."}}, page.Children...)
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, page, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Where the events are told when nobody is looking (ADR-020) opens over
	// the page from its heading line.
	var act *widget.ActionBar
	if s.alerter != nil {
		act = &widget.ActionBar{Heading: true, OpensPanel: true,
			Action: &widget.TableAction{Label: "Alerts", Href: alertsPath, Icon: "bell", Style: "quiet"}}
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Events", Tone: "neutral", HeadingAct: s.headingAct(r, act, lang, t)}, "wide",
		s.sectionPages("System", r.URL.Path), template.HTML(body.String())) //nolint:gosec // rendered by the shell's own templates
}

// eventRow says one event in words: when, on what, and what happened, with
// the weight of it as the status colour.
func eventRow(e history.Event, now time.Time, tr func(string) string) widget.TableRow {
	what, variant := e.Kind, "neutral"
	switch e.Kind {
	case "wan.down":
		what, variant = tr("Internet went down"), "danger"
	case "wan.up":
		var d struct{ After int64 }
		_ = json.Unmarshal([]byte(e.Detail), &d)
		what, variant = fmt.Sprintf(tr("Internet came back after %s"), units.Duration(d.After)), "success"
	case "boot":
		var d bootDetail
		_ = json.Unmarshal([]byte(e.Detail), &d)
		what = tr("Router started")
		if !d.Clean {
			what, variant = tr("Router started after losing power or crashing"), "warning"
		}
	case "storage.unavailable":
		var d struct{ Cause string }
		_ = json.Unmarshal([]byte(e.Detail), &d)
		what, variant = fmt.Sprintf(tr("History could not be saved: %s"), d.Cause), "warning"
	case "storage.available":
		what, variant = tr("History is saved again"), "success"
	case "changes.applied":
		what = fmt.Sprintf(tr("Changes applied: %s"), changedPages(e.Detail, tr))
	case "changes.rolledback":
		what, variant = fmt.Sprintf(tr("Changes rolled back: %s. The router could not be reached to confirm them."), changedPages(e.Detail, tr)), "warning"
	}
	row := widget.TableRow{Cells: []widget.TableCell{
		{Text: eventTime(e.Time, now), Verbatim: true},
		{Text: e.Subject},
		{Text: what, Variant: variant},
	}}
	// An apply opens its entry in config history (ADR-021).
	if e.Kind == "changes.applied" || e.Kind == "changes.rolledback" {
		var d changesDetail
		if json.Unmarshal([]byte(e.Detail), &d) == nil && d.Change > 0 {
			row.Panel = fmt.Sprintf("%s/%d", changesPath, d.Change)
			row.Cells = append(row.Cells, widget.TableCell{Actions: []widget.TableRowAct{{Icon: "info", Title: "Details", Opens: true}}})
		}
	}
	return row
}

// changedPages names the pages an apply's changes were made on, in the
// reader's language.
func changedPages(detail string, tr func(string) string) string {
	var d struct{ Pages []string }
	_ = json.Unmarshal([]byte(detail), &d)
	names := make([]string, len(d.Pages))
	for i, page := range d.Pages {
		names[i] = tr(page)
	}
	return strings.Join(names, ", ")
}

// eventMarks are the events over [start, end) as marks on its chart, oldest
// first: each outage from going down to coming back, cut at the window's
// edges and running to now while it lasts, and each boot at its moment.
func eventMarks(newestFirst []history.Event, start, end, now time.Time, tr func(string) string) []widget.ChartMark {
	span := float64(end.Sub(start))
	at := func(t time.Time) float64 { return min(1, max(0, float64(t.Sub(start))/span)) }
	var marks []widget.ChartMark
	var down *history.Event
	for i := len(newestFirst) - 1; i >= 0; i-- {
		e := newestFirst[i]
		switch e.Kind {
		case "wan.down":
			if down == nil {
				down = &newestFirst[i]
			}
		case "wan.up":
			if down != nil && e.Time.After(start) {
				marks = append(marks, widget.ChartMark{
					Kind: "outage", From: at(down.Time), To: at(e.Time),
					Label: fmt.Sprintf(tr("Internet down %s–%s"), eventTime(down.Time, now), eventTime(e.Time, now)),
				})
			}
			down = nil
		case "boot":
			if !e.Time.Before(start) && e.Time.Before(end) {
				marks = append(marks, widget.ChartMark{
					Kind: "boot", From: at(e.Time), Label: fmt.Sprintf(tr("Router started at %s"), eventTime(e.Time, now)),
				})
			}
		}
	}
	if down != nil {
		marks = append(marks, widget.ChartMark{
			Kind: "outage", From: at(down.Time), To: at(now),
			Label: fmt.Sprintf(tr("Internet down since %s"), eventTime(down.Time, now)),
		})
	}
	return marks
}

// lossThreshold is the share of the gateway's echoes a period must lose to
// be marked: a stray lost echo is noise, a twentieth is a line in trouble.
const lossThreshold = 5

// lossMarks are the stretches of a window whose periods lost lossThreshold
// percent or more of the gateway's echoes, as bands titled with the worst
// period. loss holds each period's average from start, step apart; a period
// an outage covers is the outage's.
func lossMarks(loss []float64, start time.Time, step time.Duration, outages []widget.ChartMark, now time.Time, tr func(string) string) []widget.ChartMark {
	n := float64(len(loss))
	lossy := func(i int) bool {
		if math.IsNaN(loss[i]) || loss[i] < lossThreshold {
			return false
		}
		from, to := float64(i)/n, float64(i+1)/n
		return !slices.ContainsFunc(outages, func(o widget.ChartMark) bool { return from < o.To && to > o.From })
	}
	var marks []widget.ChartMark
	for i := 0; i < len(loss); i++ {
		if !lossy(i) {
			continue
		}
		j, worst := i, loss[i]
		for j+1 < len(loss) && lossy(j+1) {
			j++
			worst = max(worst, loss[j])
		}
		from, to := start.Add(time.Duration(i)*step), start.Add(time.Duration(j+1)*step)
		marks = append(marks, widget.ChartMark{
			Kind: "loss", From: float64(i) / n, To: float64(j+1) / n,
			Label: fmt.Sprintf(tr("Packet loss up to %d %% %s–%s"), int(math.Round(worst)), eventTime(from, now), eventTime(to, now)),
		})
		i = j
	}
	return marks
}

// eventTime is the clock time today, the day and time this year, and the
// year as well before it.
func eventTime(at, now time.Time) string {
	at, now = clock.Local(at), clock.Local(now)
	switch {
	case at.YearDay() == now.YearDay() && at.Year() == now.Year():
		return at.Format("15:04")
	case at.Year() == now.Year():
		return at.Format("2 Jan, 15:04")
	}
	return at.Format("2 Jan 2006, 15:04")
}

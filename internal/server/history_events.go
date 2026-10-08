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
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/openwrt"
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

// handleEvents lists what happened, newest first (ADR-019).
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
	table := &widget.Table{
		Columns: []widget.TableColumn{
			{Label: "When", Width: widget.MeasureName},
			{Label: "Interface", Kind: "mono", Width: widget.MeasureName},
			{Label: "Event", Kind: "status"},
		},
		EmptyText: "Nothing has happened yet. The router records when the internet goes down and comes back, and when it starts.",
	}
	for _, e := range events {
		table.Rows = append(table.Rows, eventRow(e, now, tr))
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, table, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Events", Tone: "neutral"}, "wide",
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
		after := loginDuration(tr, d.After)
		if d.After < 60 {
			after = fmt.Sprintf(tr("%d s"), d.After)
		}
		what, variant = fmt.Sprintf(tr("Internet came back after %s"), after), "success"
	case "boot":
		var d bootDetail
		_ = json.Unmarshal([]byte(e.Detail), &d)
		what = tr("Router started")
		if !d.Clean {
			what, variant = tr("Router started after losing power or crashing"), "warning"
		}
	}
	return widget.TableRow{Cells: []widget.TableCell{
		{Text: eventTime(e.Time, now), Verbatim: true},
		{Text: e.Subject},
		{Text: what, Variant: variant, Verbatim: true},
	}}
}

// eventTime is the clock time today, the day and time this year, and the
// year as well before it.
func eventTime(at, now time.Time) string {
	switch {
	case at.YearDay() == now.YearDay() && at.Year() == now.Year():
		return at.Format("15:04")
	case at.Year() == now.Year():
		return at.Format("2 Jan, 15:04")
	}
	return at.Format("2 Jan 2006, 15:04")
}

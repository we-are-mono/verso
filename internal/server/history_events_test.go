// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

func wanOn(device string) openwrt.WANState {
	return openwrt.WANState{Devices: []openwrt.WANDevice{
		{Device: device, Routes: []openwrt.WANRoute{{Family: 4, Main: true}}},
	}}
}

// TestWANWatchRecordsEachChange: the first reading only sets the state; the
// internet going away is recorded against the device it left, and its return
// says how long it was gone.
func TestWANWatchRecordsEachChange(t *testing.T) {
	var w wanWatch
	at := time.Unix(1_800_000_000, 0)
	if got := w.observe(at, wanOn("wan0")); len(got) != 0 {
		t.Fatalf("first reading recorded %+v", got)
	}
	if got := w.observe(at.Add(10*time.Second), wanOn("wan0")); len(got) != 0 {
		t.Fatalf("an unchanged reading recorded %+v", got)
	}
	down := w.observe(at.Add(20*time.Second), openwrt.WANState{})
	if len(down) != 1 || down[0].Kind != "wan.down" || down[0].Subject != "wan0" || !down[0].Time.Equal(at.Add(20*time.Second)) {
		t.Fatalf("down = %+v, want wan.down on wan0", down)
	}
	up := w.observe(at.Add(8*time.Minute+20*time.Second), wanOn("wan0"))
	if len(up) != 1 || up[0].Kind != "wan.up" || up[0].Subject != "wan0" || up[0].Detail != `{"after":480}` {
		t.Fatalf("up = %+v, want wan.up on wan0 after 480 s", up)
	}
}

// TestWANWatchCountsOnlyTheMainTable: a default route in a policy table
// carries only the traffic marked for it (a VPN's table), so with the main
// table's gone the internet is down for everyone else.
func TestWANWatchCountsOnlyTheMainTable(t *testing.T) {
	var w wanWatch
	at := time.Unix(1_800_000_000, 0)
	policy := openwrt.WANDevice{Device: "eth0", Routes: []openwrt.WANRoute{{Family: 4, Table: 100}}}
	both := wanOn("wan0")
	both.Devices = append(both.Devices, policy)
	w.observe(at, both)
	down := w.observe(at.Add(10*time.Second), openwrt.WANState{Devices: []openwrt.WANDevice{policy}})
	if len(down) != 1 || down[0].Kind != "wan.down" || down[0].Subject != "wan0" {
		t.Fatalf("down = %+v, want wan.down on wan0 with only a policy route left", down)
	}
}

// TestBootIsRecordedOncePerBoot: a boot is known by the kernel's boot id, not
// its boot time, which moves when the clock is set after boot. A shell
// restarted within one boot records nothing; a new boot is recorded at the
// kernel's boot time, clean when the shell was stopped before it, unclean
// when it was not.
func TestBootIsRecordedOncePerBoot(t *testing.T) {
	booted := time.Unix(1_800_000_000, 0)
	last := history.Event{Kind: "boot", Time: booted.Add(-90 * time.Second), Detail: `{"clean":true,"id":"b-1"}`}
	if _, ok := bootEvent("b-1", booted, last, false); ok {
		t.Fatal("a shell restart within one boot recorded a boot")
	}
	got, ok := bootEvent("b-2", booted, last, true)
	if !ok || got.Kind != "boot" || got.Subject != "" || !got.Time.Equal(booted) || got.Detail != `{"clean":true,"id":"b-2"}` {
		t.Fatalf("boot after a clean stop = %+v, %v", got, ok)
	}
	got, ok = bootEvent("b-3", booted, history.Event{}, false)
	if !ok || got.Detail != `{"clean":false,"id":"b-3"}` {
		t.Fatalf("boot with no stop recorded = %+v, %v", got, ok)
	}
}

// TestBootTimeIsReadFromProcStat: btime is the kernel's boot in seconds.
func TestBootTimeIsReadFromProcStat(t *testing.T) {
	got, err := bootTime([]byte("cpu  1 2 3\nintr 5\nbtime 1800000000\nprocesses 9\n"))
	if err != nil || !got.Equal(time.Unix(1_800_000_000, 0)) {
		t.Fatalf("bootTime = %v, %v", got, err)
	}
	if _, err := bootTime([]byte("cpu 1\n")); err == nil {
		t.Fatal("a stat without btime read as a time")
	}
}

// TestEventsPageListsWhatHappenedNewestFirst: each event is a row — when, on
// which interface, and what happened as a status, coloured by its weight.
func TestEventsPageListsWhatHappenedNewestFirst(t *testing.T) {
	s := newServer(t, fakeBackend{})
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	at := time.Now().AddDate(0, 0, -3).Truncate(time.Minute)
	for _, e := range []history.Event{
		{Time: at.Add(-time.Hour), Kind: "boot", Detail: `{"clean":false,"id":"b-1"}`},
		{Time: at, Kind: "wan.down", Subject: "wan0"},
		{Time: at.Add(8 * time.Minute), Kind: "wan.up", Subject: "wan0", Detail: `{"after":480}`},
	} {
		if err := store.Record(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}

	status, body := getSignedIn(t, s, "/system/events")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	back := strings.Index(body, "Internet came back after 8 min")
	down := strings.Index(body, "Internet went down")
	boot := strings.Index(body, "Router started after losing power or crashing")
	if back < 0 || down < 0 || boot < 0 || !(back < down && down < boot) {
		t.Fatalf("events missing or out of order (back %d, down %d, boot %d):\n%s", back, down, boot, body)
	}
	for _, want := range []string{eventTime(at, time.Now()), ">wan0<", "Events"} {
		if !strings.Contains(body, want) {
			t.Errorf("events page missing %q", want)
		}
	}
}

// TestAnEventIsSaidInWords: what happened is a sentence, so it is set in sans;
// mono is for the strings a machine wrote (the interface).
func TestAnEventIsSaidInWords(t *testing.T) {
	row := eventRow(history.Event{Kind: "wan.down", Subject: "wan0"}, time.Now(), func(s string) string { return s })
	if row.Cells[2].Verbatim {
		t.Fatal("the event is set as a machine string")
	}
}

// TestAShortOutageSaysItsSeconds: under a minute reads in seconds, not as
// "0 min".
func TestAShortOutageSaysItsSeconds(t *testing.T) {
	row := eventRow(history.Event{Kind: "wan.up", Subject: "wan0", Detail: `{"after":25}`}, time.Now(), func(s string) string { return s })
	if got := row.Cells[2].Text; got != "Internet came back after 25 s" {
		t.Fatalf("event = %q", got)
	}
}

// TestEventMarksPairEachOutageWithinTheWindow: an outage runs from going down
// to coming back, cut at the window's edges; one still going runs to now; a
// boot is a moment; what fell before the window is not drawn.
func TestEventMarksPairEachOutageWithinTheWindow(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)
	start, end := now.Add(-10*time.Hour), now
	at := func(h float64) time.Time { return start.Add(time.Duration(h * float64(time.Hour))) }
	newestFirst := []history.Event{
		{Time: at(9), Kind: "wan.down", Subject: "wan0"},
		{Time: at(7.5), Kind: "boot", Detail: `{"clean":true,"id":"b"}`},
		{Time: at(6), Kind: "wan.up", Subject: "wan0"},
		{Time: at(5), Kind: "wan.down", Subject: "wan0"},
		{Time: at(1), Kind: "wan.up", Subject: "wan0"},
		{Time: at(-1), Kind: "wan.down", Subject: "wan0"},
		{Time: at(-3), Kind: "wan.up", Subject: "wan0"},
		{Time: at(-4), Kind: "wan.down", Subject: "wan0"},
		{Time: at(-5), Kind: "boot", Detail: `{"clean":true,"id":"a"}`},
	}
	got := eventMarks(newestFirst, start, end, now, func(s string) string { return s })
	want := []widget.ChartMark{
		{Kind: "outage", From: 0, To: 0.1, Label: "Internet down 01:00–03:00"},
		{Kind: "outage", From: 0.5, To: 0.6, Label: "Internet down 07:00–08:00"},
		{Kind: "boot", From: 0.75, Label: "Router started at 09:30"},
		{Kind: "outage", From: 0.9, To: 1, Label: "Internet down since 11:00"},
	}
	if len(got) != len(want) {
		t.Fatalf("marks = %+v\nwant   %+v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Kind != w.Kind || g.Label != w.Label || math.Abs(g.From-w.From) > 1e-9 || math.Abs(g.To-w.To) > 1e-9 {
			t.Errorf("mark %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestLossMarksAreRunsOfLossyPeriods: periods losing 5 % or more of the
// gateway's echoes run together into one band, titled with the worst of them;
// a gap ends a run; loss inside an outage is the outage's, so it is not drawn
// twice.
func TestLossMarksAreRunsOfLossyPeriods(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)
	start := now.Add(-10 * time.Minute)
	nan := math.NaN()
	loss := []float64{0, 6, 23, 4, nan, 50, 100, 100, 0, 0}
	outages := []widget.ChartMark{{Kind: "outage", From: 0.6, To: 0.8}}
	got := lossMarks(loss, start, time.Minute, outages, now, func(s string) string { return s })
	if len(got) != 2 {
		t.Fatalf("marks = %+v, want the 11:51–11:53 run and the 11:55–11:56 run", got)
	}
	if want := (widget.ChartMark{Kind: "loss", From: 0.1, To: 0.3, Label: "Packet loss up to 23 % 11:51–11:53"}); got[0] != want {
		t.Errorf("first = %+v, want %+v", got[0], want)
	}
	if got[1].From != 0.5 || got[1].To != 0.6 {
		t.Errorf("second = %+v, want the minute before the outage", got[1])
	}
}

// TestEventTimeSaysNoMoreThanItNeeds: today is a clock time, this year adds
// the day, an earlier year the year too.
func TestEventTimeSaysNoMoreThanItNeeds(t *testing.T) {
	now := time.Date(2026, 10, 8, 18, 0, 0, 0, time.Local)
	for at, want := range map[time.Time]string{
		time.Date(2026, 10, 8, 3, 12, 0, 0, time.Local): "03:12",
		time.Date(2026, 10, 7, 23, 5, 0, 0, time.Local): "7 Oct, 23:05",
		time.Date(2025, 12, 31, 9, 0, 0, 0, time.Local): "31 Dec 2025, 09:00",
	} {
		if got := eventTime(at, now); got != want {
			t.Errorf("eventTime(%v) = %q, want %q", at, got, want)
		}
	}
}

// TestEventsPageWithoutHistory: with no history kept there is no page to show.
func TestEventsPageWithoutHistory(t *testing.T) {
	s := newServer(t, fakeBackend{})
	if status, _ := getSignedIn(t, s, "/system/events"); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

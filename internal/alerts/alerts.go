// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package alerts tells the operator what happened while nobody was looking
// (ADR-020): incidents made from the events the shell records, one message
// each, sent to the operator's webhook with ntfy's headers, kept and retried
// in the history's outbox; and a heartbeat to an outside monitor, which
// notices the silence a router that is down cannot report.
package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/history"
)

// The kinds of alert an operator chooses from (verso.alerts.kind).
const (
	KindWAN  = "wan"  // the internet going down and coming back
	KindBoot = "boot" // a start after losing power or crashing
)

// DefaultKinds alert when verso.alerts.kind is absent.
var DefaultKinds = []string{KindWAN, KindBoot}

const (
	// flapWindow joins drops this close to a recovery into one incident.
	flapWindow = 5 * time.Minute
	// confirmDrop holds a drop before its first try, so one that ends within
	// it is told once, start and end together.
	confirmDrop = time.Minute
	// giveUp is how long an alert is tried before it is dropped.
	giveUp = 24 * time.Hour
	// retryFirst doubles with each failed attempt, up to retryMost.
	retryFirst, retryMost = 30 * time.Second, 10 * time.Minute
	// beatEvery is the heartbeat's period; the monitor's grace is set there.
	beatEvery = 5 * time.Minute
	tickEvery = 10 * time.Second
)

// Config is verso.alerts: where alerts go, where the heartbeat goes, and which
// kinds alert (nil: DefaultKinds). An empty address sends nothing.
type Config struct {
	URL       string
	Heartbeat string
	Kinds     []string
}

// incident is the internet's outage as one story: from its first drop to its
// last recovery, flapping included.
type incident struct {
	start, end time.Time
	open       bool
	downAlert  int64
	upAlert    int64
}

// Alerter turns events into alerts and sends them. Safe for concurrent use.
type Alerter struct {
	store  *history.Store
	host   string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	cfg      Config
	wan      *incident
	lastBeat time.Time
	after    int64
}

// New alerts in host's name ("home-router: the internet is down").
func New(store *history.Store, host string) *Alerter {
	return &Alerter{store: store, host: host, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
}

// Configure takes up new settings; it takes effect from the next event.
func (a *Alerter) Configure(c Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = c
}

func (a *Alerter) wants(kind string) bool {
	if a.cfg.URL == "" {
		return false
	}
	kinds := a.cfg.Kinds
	if kinds == nil {
		kinds = DefaultKinds
	}
	return slices.Contains(kinds, kind)
}

// Begin skips the events recorded before the alerter started: they happened
// while it was not watching, and were reported then or not at all.
func (a *Alerter) Begin(ctx context.Context) error {
	_, last, err := a.store.EventsAfter(ctx, 0)
	a.mu.Lock()
	a.after = last
	a.mu.Unlock()
	return err
}

// Observe makes alerts of events, oldest first.
func (a *Alerter) Observe(ctx context.Context, events []history.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, e := range events {
		var err error
		switch {
		case e.Kind == "wan.down" && a.wants(KindWAN):
			err = a.wanDown(ctx, e.Time)
		case e.Kind == "wan.up" && a.wants(KindWAN):
			err = a.wanUp(ctx, e.Time)
		case e.Kind == "boot" && a.wants(KindBoot):
			var d struct{ Clean bool }
			_ = json.Unmarshal([]byte(e.Detail), &d)
			if !d.Clean {
				_, err = a.store.Enqueue(ctx, history.Alert{
					Created: e.Time, Title: a.title("the router restarted"), Priority: "high",
					Body: fmt.Sprintf("After losing power or crashing, at %s.", clockTime(e.Time)),
				})
			}
		}
		if err != nil {
			log.Printf("verso: alert not kept: %v", err)
		}
	}
}

func (a *Alerter) wanDown(ctx context.Context, at time.Time) error {
	if in := a.wan; in != nil && (in.open || at.Sub(in.end) < flapWindow) {
		in.open = true
		if in.upAlert != 0 {
			if _, err := a.store.Cancel(ctx, in.upAlert); err != nil {
				return err
			}
			in.upAlert = 0
		}
		_, err := a.store.Rewrite(ctx, in.downAlert, a.title("the internet is down"), fmt.Sprintf("Since %s.", clockTime(in.start)))
		return err
	}
	id, err := a.store.Enqueue(ctx, history.Alert{
		Created: at, Hold: at.Add(confirmDrop), Title: a.title("the internet is down"), Priority: "high",
		Body: fmt.Sprintf("Since %s.", clockTime(at)),
	})
	a.wan = &incident{start: at, open: true, downAlert: id}
	return err
}

func (a *Alerter) wanUp(ctx context.Context, at time.Time) error {
	in := a.wan
	if in == nil || !in.open {
		return nil
	}
	in.open, in.end = false, at
	rewritten, err := a.store.Rewrite(ctx, in.downAlert, a.title("the internet was down"),
		fmt.Sprintf("From %s to %s (%s).", clockTime(in.start), clockTime(at), duration(at.Sub(in.start))))
	if err != nil || rewritten {
		return err
	}
	in.upAlert, err = a.store.Enqueue(ctx, history.Alert{
		Created: at, Title: a.title("the internet is back"), Priority: "default",
		Body: fmt.Sprintf("After %s, down since %s.", duration(at.Sub(in.start)), clockTime(in.start)),
	})
	return err
}

func (a *Alerter) title(what string) string { return a.host + ": " + what }

// Deliver sends each alert whose time has come. A refusal or no answer waits
// and tries again, longer each time; an alert a day old is given up.
func (a *Alerter) Deliver(ctx context.Context) {
	a.mu.Lock()
	url := a.cfg.URL
	a.mu.Unlock()
	now := a.now()
	due, err := a.store.Due(ctx, now)
	if err != nil {
		log.Printf("verso: alerts not read: %v", err)
		return
	}
	for _, alert := range due {
		state, result, next := "pending", "", time.Time{}
		switch {
		case now.Sub(alert.Created) > giveUp:
			state, result = "failed", "given up after a day"
		case url == "":
			state, result = "failed", "no address to send to"
		default:
			var status int
			status, result = a.post(ctx, url, alert)
			if status >= 200 && status < 300 {
				state = "sent"
			} else {
				next = now.Add(min(retryMost, retryFirst<<min(alert.Attempts, 10)))
			}
		}
		if err := a.store.Attempted(ctx, alert.ID, now, state, result, next); err != nil {
			log.Printf("verso: alert attempt not recorded: %v", err)
		}
	}
}

// post sends one alert and says how it went: the status, and as words the
// status or why there was none (status 0).
func (a *Alerter) post(ctx context.Context, url string, alert history.Alert) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(alert.Body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", alert.Title)
	if alert.Priority != "" {
		req.Header.Set("Priority", alert.Priority)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	_ = res.Body.Close()
	return res.StatusCode, strconv.Itoa(res.StatusCode)
}

// Configured is whether alerts have an address to go to.
func (a *Alerter) Configured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.URL != ""
}

// Test posts a test alert at once and says why it did not arrive, if it did
// not: no address, no answer, or the receiver's refusal by status.
func (a *Alerter) Test(ctx context.Context) error {
	a.mu.Lock()
	url := a.cfg.URL
	a.mu.Unlock()
	if url == "" {
		return errors.New("no alert address is set")
	}
	status, result := a.post(ctx, url, history.Alert{Title: a.title("a test alert"), Body: "If you can read this, alerts reach you."})
	if status < 200 || status >= 300 {
		return fmt.Errorf("the receiver answered %s", result)
	}
	return nil
}

// Beat tells the outside monitor the router is alive, once every five
// minutes.
func (a *Alerter) Beat(ctx context.Context) {
	a.mu.Lock()
	url, now := a.cfg.Heartbeat, a.now()
	if url == "" || now.Sub(a.lastBeat) < beatEvery {
		a.mu.Unlock()
		return
	}
	a.lastBeat = now
	a.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("verso: heartbeat: %v", err)
		return
	}
	res, err := a.client.Do(req)
	if err != nil {
		return
	}
	_ = res.Body.Close()
}

// Run reads new events, sends what is due and beats, every ten seconds, until
// ctx ends.
func (a *Alerter) Run(ctx context.Context) {
	tick := time.NewTicker(tickEvery)
	defer tick.Stop()
	for {
		a.mu.Lock()
		after := a.after
		a.mu.Unlock()
		events, last, err := a.store.EventsAfter(ctx, after)
		if err == nil {
			a.Observe(ctx, events)
			a.mu.Lock()
			a.after = last
			a.mu.Unlock()
		}
		a.Deliver(ctx)
		a.Beat(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func clockTime(t time.Time) string { return clock.Local(t).Format("15:04") }

// duration says how long, as the Events page does: seconds under a minute,
// minutes under an hour, then hours and minutes.
func duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%d h %02d min", int(d.Hours()), int(d.Minutes())%60)
}

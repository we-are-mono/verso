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
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/units"
)

// The router's own kinds of alert an operator chooses from (verso.alerts.kind),
// beside one for each installed plugin that declares an alerting event, named
// by its id (ADR-028).
const (
	KindWAN      = "wan"      // the internet going down and coming back
	KindBoot     = "boot"     // a start after losing power or crashing
	KindRollback = "rollback" // applied changes the router undid, unconfirmed
	KindStorage  = "storage"  // the history not being saved
)

// Every is every kind of alert: the router's own and, in the order table found
// them, each installed plugin's that declares an alerting event. They all
// alert while verso.alerts.kind is absent.
func Every(table *events.Table) []string {
	kinds := []string{KindWAN, KindBoot, KindRollback, KindStorage}
	if table != nil {
		for _, p := range table.AlertPlugins() {
			kinds = append(kinds, p.ID)
		}
	}
	return kinds
}

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
// kinds alert (nil: Every kind). Table is the kinds of event the router
// records, its own and the installed plugins' (nil: the router's own). An
// empty address sends nothing.
type Config struct {
	URL       string
	Heartbeat string
	Kinds     []string
	Table     *events.Table
}

// incident is one story told as one alert and its recovery: from the first
// event of its opening kind on its subject to the last that closes it, a
// flapping line's drops included (ADR-020). first is the event that opened it,
// whose words its alert says.
type incident struct {
	first      history.Event
	kind       events.Kind
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

	mu        sync.Mutex
	cfg       Config
	incidents map[string]*incident
	lastBeat  time.Time
	after     int64
}

// New alerts in host's name ("home-router: the internet is down").
func New(store *history.Store, host string) *Alerter {
	return &Alerter{store: store, host: host, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
		incidents: map[string]*incident{}}
}

// Configure takes up new settings; it takes effect from the next event.
func (a *Alerter) Configure(c Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = c
}

// SetTable takes up the kinds of event the installed plugins declare now,
// after a plugin came or went, keeping every other setting.
func (a *Alerter) SetTable(t *events.Table) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Table = t
}

// routersOwn is the table of the router's own kinds, for an alerter told of
// no plugins.
var routersOwn = events.NewTable(nil)

func (a *Alerter) table() *events.Table {
	if a.cfg.Table == nil {
		return routersOwn
	}
	return a.cfg.Table
}

func (a *Alerter) wants(kind string) bool {
	if a.cfg.URL == "" || kind == "" {
		return false
	}
	kinds := a.cfg.Kinds
	if kinds == nil {
		kinds = Every(a.cfg.Table)
	}
	return slices.Contains(kinds, kind)
}

// Begin takes up the incidents the history leaves open, and skips the events
// recorded before the alerter started: they happened while it was not
// watching, and were reported then or not at all. Their incidents are taken
// up as told: a repeat of one still open sends nothing, and its close sends
// the recovery.
func (a *Alerter) Begin(ctx context.Context) error {
	recorded, last, err := a.store.EventsAfter(ctx, 0)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.observe(ctx, recorded, true)
	a.after = last
	return err
}

// Observe makes alerts of events, oldest first.
func (a *Alerter) Observe(ctx context.Context, recorded []history.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.observe(ctx, recorded, false)
}

// observe makes alerts of events, oldest first; told, it only takes their
// incidents up, as Begin does, keeping nothing.
func (a *Alerter) observe(ctx context.Context, recorded []history.Event, told bool) {
	table := a.table()
	for _, e := range recorded {
		k := table.Of(e)
		var err error
		switch {
		case len(k.Ends) > 0:
			err = a.closed(ctx, k, e, told)
		case k.Unknown || !a.wants(k.Alert):
		case table.Ended(k.Name):
			err = a.opened(ctx, k, e, told)
		case told:
		case e.Kind == "changes.rolledback":
			var d struct{ Pages []string }
			_ = json.Unmarshal([]byte(e.Detail), &d)
			_, err = a.store.Enqueue(ctx, history.Alert{
				Created: e.Time, Title: a.title("changes were rolled back"), Priority: "high",
				Body: fmt.Sprintf("Changes to %s were undone at %s: the router could not be reached to confirm them.",
					strings.Join(d.Pages, ", "), clockTime(e.Time)),
			})
		case e.Kind == "boot":
			var d struct{ Clean bool }
			_ = json.Unmarshal([]byte(e.Detail), &d)
			if !d.Clean {
				_, err = a.store.Enqueue(ctx, history.Alert{
					Created: e.Time, Title: a.title("the router restarted"), Priority: "high",
					Body: fmt.Sprintf("After losing power or crashing, at %s.", clockTime(e.Time)),
				})
			}
		case k.Plugin != "":
			what, tone := k.Words(e, english)
			_, err = a.store.Enqueue(ctx, history.Alert{
				Created: e.Time, Title: a.title(what), Priority: priority(tone),
				Body: fmt.Sprintf("At %s.", clockTime(e.Time)),
			})
		}
		if err != nil {
			log.Printf("verso: alert not kept: %v", err)
		}
	}
}

// opened opens the incident e starts, or reopens one closed less than
// flapWindow ago, withdrawing its recovery if that is not sent yet. Its first
// alert waits confirmDrop, so one closed sooner is told once.
func (a *Alerter) opened(ctx context.Context, k events.Kind, e history.Event, told bool) error {
	key := k.Name + "\x00" + e.Subject
	if in := a.incidents[key]; in != nil && (in.open || e.Time.Sub(in.end) < flapWindow) {
		in.open = true
		if told {
			return nil
		}
		if in.upAlert != 0 {
			if _, err := a.store.Cancel(ctx, in.upAlert); err != nil {
				return err
			}
			in.upAlert = 0
		}
		title, body := a.openedWords(in)
		_, err := a.store.Rewrite(ctx, in.downAlert, title, body)
		return err
	}
	in := &incident{first: e, kind: k, start: e.Time, open: true}
	a.incidents[key] = in
	if told {
		return nil
	}
	title, body := a.openedWords(in)
	_, tone := k.Words(e, english)
	id, err := a.store.Enqueue(ctx, history.Alert{
		Created: e.Time, Hold: e.Time.Add(confirmDrop), Title: title, Body: body, Priority: priority(tone),
	})
	in.downAlert = id
	return err
}

// closed closes every open incident e's kind ends on e's subject: its first
// alert, still waiting, becomes one message from start to end; one already
// sent is followed by a recovery naming how long it lasted.
func (a *Alerter) closed(ctx context.Context, k events.Kind, e history.Event, told bool) error {
	for _, end := range k.Ends {
		in := a.incidents[end+"\x00"+e.Subject]
		if in == nil || !in.open {
			continue
		}
		in.open, in.end = false, e.Time
		if told {
			continue
		}
		title, body := a.foldedWords(in)
		rewritten, err := a.store.Rewrite(ctx, in.downAlert, title, body)
		if err != nil {
			return err
		}
		if rewritten {
			continue
		}
		title, body = a.recoveredWords(in, k, e)
		if in.upAlert, err = a.store.Enqueue(ctx, history.Alert{Created: e.Time, Title: title, Body: body, Priority: "default"}); err != nil {
			return err
		}
	}
	return nil
}

// The words of an incident's alerts: the internet's, as ADR-020 has them, and
// a plugin's, in the sentences of its opening and its closing events.

func (a *Alerter) openedWords(in *incident) (title, body string) {
	what := "the internet is down"
	if in.kind.Plugin != "" {
		what, _ = in.kind.Words(in.first, english)
	}
	return a.title(what), fmt.Sprintf("Since %s.", clockTime(in.start))
}

func (a *Alerter) foldedWords(in *incident) (title, body string) {
	what := "the internet was down"
	if in.kind.Plugin != "" {
		what, _ = in.kind.Words(in.first, english)
	}
	return a.title(what), fmt.Sprintf("From %s to %s (%s).", clockTime(in.start), clockTime(in.end), duration(in.end.Sub(in.start)))
}

func (a *Alerter) recoveredWords(in *incident, k events.Kind, e history.Event) (title, body string) {
	lasted := duration(e.Time.Sub(in.start))
	if in.kind.Plugin == "" {
		return a.title("the internet is back"), fmt.Sprintf("After %s, down since %s.", lasted, clockTime(in.start))
	}
	what, _ := k.Words(e, english)
	return a.title(what), fmt.Sprintf("After %s, since %s.", lasted, clockTime(in.start))
}

// english is the translator of an alert: none, since alerts are said in
// English (ADR-020).
func english(s string) string { return s }

// priority is ntfy's priority for an alert in tone.
func priority(tone string) string {
	if tone == "danger" || tone == "warning" {
		return "high"
	}
	return "default"
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
		return 0, withoutAddress(err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", alert.Title)
	if alert.Priority != "" {
		req.Header.Set("Priority", alert.Priority)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return 0, withoutAddress(err)
	}
	_ = res.Body.Close()
	return res.StatusCode, strconv.Itoa(res.StatusCode)
}

// withoutAddress is why a request to an alert address failed, without the
// address: Go's client names it in every error it returns, and an alert
// address is a secret (internal/secrets) that what an attempt keeps, a test's
// answer and the log never carry.
func withoutAddress(err error) string {
	var failed *neturl.Error
	if errors.As(err, &failed) {
		return failed.Err.Error()
	}
	return err.Error()
}

// Urgent sends an alert of kind at once, once, if that kind was chosen. It
// is for news of the history the outbox lives in: its not being saved, which
// the outbox cannot hold, or its file not being rewritten. An alert that does
// not arrive is not tried again.
func (a *Alerter) Urgent(ctx context.Context, kind, what, body string) {
	a.mu.Lock()
	url, wanted := a.cfg.URL, a.wants(kind)
	a.mu.Unlock()
	if !wanted {
		return
	}
	if status, result := a.post(ctx, url, history.Alert{Title: a.title(what), Body: body, Priority: "high"}); status < 200 || status >= 300 {
		log.Printf("verso: alert %q did not arrive: %s", what, result)
	}
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
		log.Printf("verso: heartbeat: %s", withoutAddress(err))
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

// duration says how long, as every duration is said (internal/units).
func duration(d time.Duration) string { return units.Duration(int64(d.Seconds())) }

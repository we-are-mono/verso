// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package alerts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

var ctx = context.Background()

var at = func(clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-08 "+clock, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func newAlerter(t *testing.T, kinds ...string) (*Alerter, *history.Store) {
	t.Helper()
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := New(store, "home-router")
	a.Configure(Config{URL: "http://receiver.invalid/alerts", Kinds: kinds})
	return a, store
}

func waiting(t *testing.T, store *history.Store) []history.Alert {
	t.Helper()
	due, err := store.Due(ctx, at("23:59:59"))
	if err != nil {
		t.Fatal(err)
	}
	return due
}

func down(clock string) history.Event {
	return history.Event{Time: at(clock), Kind: "wan.down", Subject: "wan0"}
}

func up(clock string) history.Event {
	return history.Event{Time: at(clock), Kind: "wan.up", Subject: "wan0"}
}

// TestAnOutageThatEndsBeforeItIsSentIsOneMessage: the internet going down is
// queued at once, and when it comes back before that could be sent, the one
// message says when it started and ended.
func TestAnOutageThatEndsBeforeItIsSentIsOneMessage(t *testing.T) {
	a, store := newAlerter(t)
	a.Observe(ctx, []history.Event{down("03:12:00")})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: the internet is down" || queued[0].Body != "Since 03:12." || queued[0].Priority != "high" {
		t.Fatalf("on going down = %+v", queued)
	}
	a.Observe(ctx, []history.Event{up("03:20:00")})
	queued = waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: the internet was down" || queued[0].Body != "From 03:12 to 03:20 (8 min)." {
		t.Fatalf("on coming back = %+v, want the one message rewritten", queued)
	}
}

// TestAnOutageAlreadyReportedIsClosedByASecondMessage.
func TestAnOutageAlreadyReportedIsClosedByASecondMessage(t *testing.T) {
	a, store := newAlerter(t)
	a.Observe(ctx, []history.Event{down("03:12:00")})
	sent := waiting(t, store)[0]
	if err := store.Attempted(ctx, sent.ID, at("03:12:05"), "sent", "200", time.Time{}); err != nil {
		t.Fatal(err)
	}
	a.Observe(ctx, []history.Event{up("04:17:30")})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: the internet is back" || queued[0].Body != "After 1 h 05 min, down since 03:12." || queued[0].Priority != "default" {
		t.Fatalf("recovery = %+v", queued)
	}
}

// TestAFlappingLineIsOneIncident: going down again within five minutes of
// coming back reopens the incident; the recovery not yet sent is withdrawn,
// and the next one counts from the first drop.
func TestAFlappingLineIsOneIncident(t *testing.T) {
	a, store := newAlerter(t)
	a.Observe(ctx, []history.Event{down("03:00:00")})
	first := waiting(t, store)[0]
	if err := store.Attempted(ctx, first.ID, at("03:00:05"), "sent", "200", time.Time{}); err != nil {
		t.Fatal(err)
	}
	a.Observe(ctx, []history.Event{up("03:01:00"), down("03:03:00"), up("03:04:00"), down("03:06:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("a flapping line queued %+v", queued)
	}
	a.Observe(ctx, []history.Event{up("03:30:00")})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Body != "After 30 min, down since 03:00." {
		t.Fatalf("recovery after flapping = %+v", queued)
	}
	a.Observe(ctx, []history.Event{down("03:40:00")})
	if queued := waiting(t, store); len(queued) != 2 {
		t.Fatalf("a drop ten minutes after recovering is a new incident; queued %+v", queued)
	}
}

// TestOnlyAnUncleanStartAlerts: a reboot the operator asked for is not news.
func TestOnlyAnUncleanStartAlerts(t *testing.T) {
	a, store := newAlerter(t)
	a.Observe(ctx, []history.Event{
		{Time: at("13:00:00"), Kind: "boot", Detail: `{"clean":true,"id":"a"}`},
		{Time: at("13:49:00"), Kind: "boot", Detail: `{"clean":false,"id":"b"}`},
	})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: the router restarted" || queued[0].Body != "After losing power or crashing, at 13:49." {
		t.Fatalf("boots = %+v", queued)
	}
}

// TestOnlyChosenKindsAlert, and nothing is queued with nowhere to send it.
func TestOnlyChosenKindsAlert(t *testing.T) {
	a, store := newAlerter(t, KindBoot)
	a.Observe(ctx, []history.Event{down("03:00:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("an unchosen kind queued %+v", queued)
	}
	a.Configure(Config{})
	a.Observe(ctx, []history.Event{{Time: at("13:49:00"), Kind: "boot", Detail: `{"clean":false,"id":"b"}`}})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("queued with no address to send to: %+v", queued)
	}
}

type receiver struct {
	mu       sync.Mutex
	status   int
	requests []*http.Request
	bodies   []string
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	rc.requests = append(rc.requests, r)
	rc.bodies = append(rc.bodies, string(body))
	w.WriteHeader(rc.status)
}

// TestDeliverPostsEachDueAlertAndRetriesAFailure: the body is the message,
// the title and priority ride ntfy's headers; a refusal waits and tries
// again; after a day the alert is given up.
func TestDeliverPostsEachDueAlertAndRetriesAFailure(t *testing.T) {
	rc := &receiver{status: http.StatusInternalServerError}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	a, store := newAlerter(t)
	a.Configure(Config{URL: srv.URL + "/topic"})
	clock := at("03:12:00")
	a.now = func() time.Time { return clock }
	a.Observe(ctx, []history.Event{down("03:12:00")})

	// A drop is held a minute, so one that ends sooner is one message.
	a.Deliver(ctx)
	if len(rc.requests) != 0 {
		t.Fatal("a drop was sent before it had lasted a minute")
	}
	clock = clock.Add(time.Minute)
	a.Deliver(ctx)
	if len(rc.requests) != 1 || rc.requests[0].Method != http.MethodPost || rc.requests[0].Header.Get("Title") != "home-router: the internet is down" ||
		rc.requests[0].Header.Get("Priority") != "high" || rc.bodies[0] != "Since 03:12." {
		t.Fatalf("request = %+v %q", rc.requests, rc.bodies)
	}
	a.Deliver(ctx)
	if len(rc.requests) != 1 {
		t.Fatal("a refused alert was retried at once")
	}
	rc.status = http.StatusOK
	clock = clock.Add(time.Minute)
	a.Deliver(ctx)
	if len(rc.requests) != 2 {
		t.Fatal("a refused alert was not retried after its wait")
	}
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("a delivered alert still waits: %+v", queued)
	}

	rc.status = http.StatusInternalServerError
	a.Observe(ctx, []history.Event{up("03:30:00"), down("04:00:00")})
	clock = clock.Add(25 * time.Hour)
	a.Deliver(ctx)
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("an alert a day old is still tried: %+v", queued)
	}
}

// TestTestSaysWhetherAlertsArrive: the operator's "Send a test alert" posts
// at once and answers with what the receiver said.
func TestTestSaysWhetherAlertsArrive(t *testing.T) {
	rc := &receiver{status: http.StatusOK}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	a, _ := newAlerter(t)
	a.Configure(Config{})
	if err := a.Test(ctx); err == nil {
		t.Fatal("a test with no address succeeded")
	}
	a.Configure(Config{URL: srv.URL})
	if err := a.Test(ctx); err != nil || len(rc.requests) != 1 || rc.requests[0].Header.Get("Title") != "home-router: a test alert" {
		t.Fatalf("test = %v, requests %d", err, len(rc.requests))
	}
	rc.status = http.StatusForbidden
	if err := a.Test(ctx); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a refused test = %v, want the 403 named", err)
	}
}

// TestBeatTellsTheMonitorTheRouterIsAlive: a GET every five minutes, none
// without an address.
func TestBeatTellsTheMonitorTheRouterIsAlive(t *testing.T) {
	rc := &receiver{status: http.StatusOK}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	a, _ := newAlerter(t)
	clock := at("03:00:00")
	a.now = func() time.Time { return clock }
	a.Beat(ctx)
	if len(rc.requests) != 0 {
		t.Fatal("beat with no heartbeat address")
	}
	a.Configure(Config{Heartbeat: srv.URL + "/ping/uuid"})
	a.Beat(ctx)
	clock = clock.Add(time.Minute)
	a.Beat(ctx)
	clock = clock.Add(5 * time.Minute)
	a.Beat(ctx)
	if len(rc.requests) != 2 || rc.requests[0].Method != http.MethodGet || rc.requests[0].URL.Path != "/ping/uuid" {
		t.Fatalf("beats = %d, want two GETs five minutes apart", len(rc.requests))
	}
}

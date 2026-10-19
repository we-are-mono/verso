// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package alerts

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// acmeKinds is acme's events: two successes, each ending both failures, the
// failures alerting, and one that alerts and nothing ends.
func acmeKinds() *events.Table {
	return events.NewTable([]plugin.Manifest{{ID: "acme", Name: "Trusted certificate", Commands: &plugin.Registry{
		Commands: []plugin.RegisteredCommand{{Name: "state", Kind: "read", Version: 1}},
		Events: []plugin.EventDeclaration{
			{Name: "issued", Sentence: "A certificate for %s was issued.", Tone: "success", Read: "state", Ends: []string{"issue-failed", "renew-failed"}},
			{Name: "renewed", Sentence: "The certificate for %s was renewed.", Tone: "success", Read: "state", Ends: []string{"renew-failed", "issue-failed"}},
			{Name: "issue-failed", Sentence: "A certificate for %s could not be issued.", Tone: "danger", Read: "state", Alert: true},
			{Name: "renew-failed", Sentence: "The certificate for %s could not be renewed.", Tone: "danger", Read: "state", Alert: true},
			{Name: "expiring", Sentence: "The certificate for %s expires soon.", Tone: "warning", Read: "state", Alert: true},
			{Name: "checked", Sentence: "Checked %s.", Tone: "info", Read: "state"},
		},
	}}})
}

// acmeAlerter alerts every kind of acme's.
func acmeAlerter(t *testing.T, kinds ...string) (*Alerter, *history.Store) {
	t.Helper()
	a, store := newAlerter(t)
	a.Configure(Config{URL: "http://receiver.invalid/alerts", Kinds: kinds, Table: acmeKinds()})
	return a, store
}

// acme is one of acme's events about router.example.com, as recorded.
func acme(event, clock string) history.Event {
	sentences := map[string]string{
		"issued": "A certificate for %s was issued.", "renewed": "The certificate for %s was renewed.",
		"issue-failed": "A certificate for %s could not be issued.", "renew-failed": "The certificate for %s could not be renewed.",
		"expiring": "The certificate for %s expires soon.", "checked": "Checked %s.",
	}
	return history.Event{Time: at(clock), Kind: "acme." + event,
		Detail: fmt.Sprintf(`{"spool":"s","plugin":"acme","sentence":%q,"args":["router.example.com"],"tone":"danger","read":"state"}`, sentences[event])}
}

func sendAll(t *testing.T, store *history.Store) {
	t.Helper()
	for _, alert := range waiting(t, store) {
		if err := store.Attempted(ctx, alert.ID, alert.Created, "sent", "200", time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAFailureThatRecoversWithinTheMinuteIsOneMessage: the failure is held a
// minute, and a success ending it within it rewrites it into one message
// saying when it began and ended.
func TestAFailureThatRecoversWithinTheMinuteIsOneMessage(t *testing.T) {
	a, store := acmeAlerter(t)
	a.Observe(ctx, []history.Event{acme("renew-failed", "03:12:00")})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: The certificate for router.example.com could not be renewed." ||
		queued[0].Body != "Since 03:12." || queued[0].Priority != "high" {
		t.Fatalf("on failing = %+v", queued)
	}
	a.Observe(ctx, []history.Event{acme("renewed", "03:12:30")})
	queued = waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: The certificate for router.example.com could not be renewed." ||
		queued[0].Body != "From 03:12 to 03:12 ("+duration(30*time.Second)+")." {
		t.Fatalf("on recovering within the minute = %+v, want the one message rewritten", queued)
	}
}

// TestAFailureThatLastsAlertsOnceAndOnceMoreWhenItEnds: repeats while it is
// open send nothing; the success that ends it sends one message naming how
// long it lasted, in its own words.
func TestAFailureThatLastsAlertsOnceAndOnceMoreWhenItEnds(t *testing.T) {
	a, store := acmeAlerter(t)
	a.Observe(ctx, []history.Event{acme("renew-failed", "03:12:00")})
	sendAll(t, store)
	a.Observe(ctx, []history.Event{acme("renew-failed", "04:12:00"), acme("renew-failed", "05:12:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("repeats while open queued %+v", queued)
	}
	a.Observe(ctx, []history.Event{acme("renewed", "06:12:00")})
	queued := waiting(t, store)
	if len(queued) != 1 || queued[0].Title != "home-router: The certificate for router.example.com was renewed." ||
		queued[0].Body != "After "+duration(3*time.Hour)+", since 03:12." || queued[0].Priority != "default" {
		t.Fatalf("recovery = %+v", queued)
	}
}

// TestEitherSuccessClosesAFailure: a failed renewal closes when a re-applied
// setup issues a certificate, and a later failure alerts again.
func TestEitherSuccessClosesAFailure(t *testing.T) {
	a, store := acmeAlerter(t)
	a.Observe(ctx, []history.Event{acme("renew-failed", "03:00:00")})
	sendAll(t, store)
	a.Observe(ctx, []history.Event{acme("issued", "09:00:00")})
	if queued := waiting(t, store); len(queued) != 1 || queued[0].Title != "home-router: A certificate for router.example.com was issued." {
		t.Fatalf("issued = %+v", queued)
	}
	sendAll(t, store)
	a.Observe(ctx, []history.Event{acme("renew-failed", "20:00:00")})
	if queued := waiting(t, store); len(queued) != 1 {
		t.Fatalf("a failure after the close = %+v, want it to alert again", queued)
	}
}

// TestAnAlertingEventNothingEndsAlertsEachTime: one message each, at its time.
func TestAnAlertingEventNothingEndsAlertsEachTime(t *testing.T) {
	a, store := acmeAlerter(t)
	a.Observe(ctx, []history.Event{acme("expiring", "08:00:00"), acme("expiring", "20:00:00")})
	queued := waiting(t, store)
	if len(queued) != 2 || queued[0].Title != "home-router: The certificate for router.example.com expires soon." || queued[0].Body != "At 08:00." {
		t.Fatalf("queued = %+v", queued)
	}
}

// TestQuietOrUnchosenEventsSendNothing: an event that does not alert, and a
// plugin the operator did not choose, send nothing.
func TestQuietOrUnchosenEventsSendNothing(t *testing.T) {
	a, store := acmeAlerter(t)
	a.Observe(ctx, []history.Event{acme("checked", "08:00:00"), acme("renewed", "08:01:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("quiet events queued %+v", queued)
	}
	unchosen, store := acmeAlerter(t, KindWAN, KindBoot)
	unchosen.Observe(ctx, []history.Event{acme("renew-failed", "08:00:00"), acme("expiring", "08:01:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("an unchosen plugin queued %+v", queued)
	}
}

// TestEveryKindIsTheRoutersAndTheAlertingPlugins: with no kinds chosen, every
// kind alerts: the router's four, and each installed plugin that declares an
// alerting event, by its id.
func TestEveryKindIsTheRoutersAndTheAlertingPlugins(t *testing.T) {
	if got := Every(acmeKinds()); !slices.Equal(got, []string{KindWAN, KindBoot, KindRollback, KindStorage, "acme"}) {
		t.Errorf("every kind = %v", got)
	}
	if got := Every(nil); !slices.Equal(got, []string{KindWAN, KindBoot, KindRollback, KindStorage}) {
		t.Errorf("with no plugins = %v", got)
	}
}

// TestOpenIncidentsOutlastARestart: an alerter started over a history holding
// an open outage and an open failure takes both up as they were: a repeat
// sends nothing, and each close sends its recovery. A closed incident is not
// taken up.
func TestOpenIncidentsOutlastARestart(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, e := range []history.Event{down("01:00:00"), up("01:10:00"), down("03:00:00"), acme("renew-failed", "03:05:00")} {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	a := New(store, "home-router")
	a.Configure(Config{URL: "http://receiver.invalid/alerts", Table: acmeKinds()})
	if err := a.Begin(ctx); err != nil {
		t.Fatal(err)
	}
	a.Observe(ctx, []history.Event{down("03:30:00"), acme("renew-failed", "04:05:00")})
	if queued := waiting(t, store); len(queued) != 0 {
		t.Fatalf("repeats after a restart queued %+v", queued)
	}
	a.Observe(ctx, []history.Event{up("04:00:00"), acme("renewed", "05:05:00")})
	queued := waiting(t, store)
	if len(queued) != 2 || queued[0].Title != "home-router: the internet is back" || queued[0].Body != "After "+duration(time.Hour)+", down since 03:00." ||
		queued[1].Title != "home-router: The certificate for router.example.com was renewed." {
		t.Fatalf("recoveries after a restart = %+v", queued)
	}
}

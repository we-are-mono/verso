// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"path/filepath"
	"testing"
	"time"
)

// TestEventsAfterHandsOverEachEventOnce: a reader that remembers the last id
// it saw gets only what was recorded since, oldest first.
func TestEventsAfterHandsOverEachEventOnce(t *testing.T) {
	store, _ := open(t)
	_, last, err := store.EventsAfter(ctx, 0)
	if err != nil || last != 0 {
		t.Fatalf("EventsAfter on nothing = last %d, %v", last, err)
	}
	first := Event{Time: minute(0), Kind: "wan.down", Subject: "wan0"}
	if err := store.Record(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, last, err := store.EventsAfter(ctx, 0)
	if err != nil || len(got) != 1 || got[0] != first {
		t.Fatalf("EventsAfter(0) = %+v, %v", got, err)
	}
	second := Event{Time: minute(1), Kind: "wan.up", Subject: "wan0"}
	if err := store.Record(ctx, second); err != nil {
		t.Fatal(err)
	}
	got, _, err = store.EventsAfter(ctx, last)
	if err != nil || len(got) != 1 || got[0] != second {
		t.Fatalf("EventsAfter(%d) = %+v, %v; want only the second", last, got, err)
	}
}

// TestAnAlertWaitsUntilItIsDue: an enqueued alert is due at once; one that
// failed waits for its next try; one delivered or given up is never due again.
func TestAnAlertWaitsUntilItIsDue(t *testing.T) {
	store, _ := open(t)
	id, err := store.Enqueue(ctx, Alert{Created: minute(0), Title: "router: Internet down", Body: "since 03:12", Priority: "high"})
	if err != nil {
		t.Fatal(err)
	}
	due, err := store.Due(ctx, minute(0))
	if err != nil || len(due) != 1 || due[0].ID != id || due[0].Body != "since 03:12" || due[0].Priority != "high" {
		t.Fatalf("Due = %+v, %v", due, err)
	}
	if err := store.Attempted(ctx, id, minute(0), "pending", "connection refused", minute(2)); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.Due(ctx, minute(1)); len(due) != 0 {
		t.Fatalf("an alert was due before its next try: %+v", due)
	}
	if due, _ := store.Due(ctx, minute(2)); len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("Due at the next try = %+v, want one attempt made", due)
	}
	if err := store.Attempted(ctx, id, minute(2), "sent", "200", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.Due(ctx, minute(60)); len(due) != 0 {
		t.Fatalf("a sent alert was due again: %+v", due)
	}
}

// TestOnlyAPendingAlertCanBeRewrittenOrCancelled: once an alert has gone out,
// what it said stands.
func TestOnlyAPendingAlertCanBeRewrittenOrCancelled(t *testing.T) {
	store, _ := open(t)
	waiting, _ := store.Enqueue(ctx, Alert{Created: minute(0), Title: "down", Body: "since 03:12"})
	sent, _ := store.Enqueue(ctx, Alert{Created: minute(0), Title: "down", Body: "since 04:00"})
	if err := store.Attempted(ctx, sent, minute(0), "sent", "200", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.Rewrite(ctx, waiting, "was down", "03:12–03:20"); err != nil || !ok {
		t.Fatalf("rewriting a waiting alert = %v, %v", ok, err)
	}
	if ok, _ := store.Rewrite(ctx, sent, "was down", "04:00–04:10"); ok {
		t.Fatal("a sent alert was rewritten")
	}
	due, _ := store.Due(ctx, minute(0))
	if len(due) != 1 || due[0].Title != "was down" || due[0].Body != "03:12–03:20" {
		t.Fatalf("Due = %+v, want the rewritten alert", due)
	}
	if ok, err := store.Cancel(ctx, waiting); err != nil || !ok {
		t.Fatalf("cancelling a waiting alert = %v, %v", ok, err)
	}
	if due, _ := store.Due(ctx, minute(0)); len(due) != 0 {
		t.Fatalf("a cancelled alert was due: %+v", due)
	}
}

// TestTheOutboxJoinsAnExistingFile: a history kept before alerts existed
// gains the outbox and keeps its points.
func TestTheOutboxJoinsAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if _, err := store.db.Exec(`DROP TABLE alerts`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if points, _ := reopened.Points(ctx, "cpu", Minute, minute(0), minute(1)); len(points) != 1 {
		t.Fatal("reopening for the outbox lost the points")
	}
	if _, err := reopened.Enqueue(ctx, Alert{Created: minute(0), Title: "t"}); err != nil {
		t.Fatalf("no outbox after reopening: %v", err)
	}
}

// TestMaintainForgetsOldAlerts: the outbox keeps thirty days.
func TestMaintainForgetsOldAlerts(t *testing.T) {
	store, _ := open(t)
	if _, err := store.Enqueue(ctx, Alert{Created: minute(0), Title: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, minute(31*24*60)); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.Due(ctx, minute(31*24*60)); len(due) != 0 {
		t.Fatalf("a month-old alert is still kept: %+v", due)
	}
}

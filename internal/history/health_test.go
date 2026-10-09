// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"errors"
	"testing"
	"time"
)

// TestHealthTurnsOnTheFirstFailureAndTheFirstSuccessAfter: a run of failed
// writes is one failure, told once with its cause and since when; the first
// write that succeeds again ends it, told once.
func TestHealthTurnsOnTheFirstFailureAndTheFirstSuccessAfter(t *testing.T) {
	store, _ := open(t)
	if h := store.Health(); !h.OK {
		t.Fatalf("a new store is unhealthy: %+v", h)
	}
	var told []Health
	store.OnHealth(func(was, now Health) { told = append(told, now) })

	full := errors.New("database or disk is full")
	store.note(full, minute(0))
	store.note(full, minute(1))
	if h := store.Health(); h.OK || h.Cause != "database or disk is full" || !h.Since.Equal(minute(0)) {
		t.Fatalf("after failing = %+v", h)
	}
	store.note(nil, minute(5))
	store.note(nil, minute(6))
	if h := store.Health(); !h.OK || !h.Since.Equal(minute(5)) {
		t.Fatalf("after recovering = %+v", h)
	}
	if len(told) != 2 || told[0].OK || !told[1].OK {
		t.Fatalf("told = %+v, want the failure and the recovery once each", told)
	}
}

// TestRecoveryIsToldWithWhatItRecoveredFrom: the listener learns when the
// failure began and why, to record it once writing works again.
func TestRecoveryIsToldWithWhatItRecoveredFrom(t *testing.T) {
	store, _ := open(t)
	var was Health
	store.OnHealth(func(before, now Health) {
		if now.OK {
			was = before
		}
	})
	store.note(errors.New("disk I/O error"), minute(0))
	store.note(nil, minute(3))
	if was.OK || was.Cause != "disk I/O error" || !was.Since.Equal(minute(0)) {
		t.Fatalf("recovered from %+v", was)
	}
}

// TestAHistoryInMemoryNeverReadsAsSaved: every write to memory succeeds, and
// saves nothing.
func TestAHistoryInMemoryNeverReadsAsSaved(t *testing.T) {
	store, _ := open(t)
	store.Fail("/data/verso cannot be written", minute(0))
	store.note(nil, minute(3))
	if h := store.Health(); h.OK || h.Cause != "/data/verso cannot be written" {
		t.Fatalf("health = %+v, want the failure to stand", h)
	}
}

// TestAFailedFlushIsNoted: a write the file refuses turns the health.
func TestAFailedFlushIsNoted(t *testing.T) {
	store, _ := open(t)
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if _, err := store.db.Exec(`DROP TABLE points`); err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(ctx); err == nil {
		t.Fatal("a flush into no table succeeded")
	}
	if h := store.Health(); h.OK || h.Cause == "" || time.Since(h.Since) > time.Minute {
		t.Fatalf("health after a failed flush = %+v", h)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// TestStorageThatRecoversRecordsWhenItFailedAndWhenItCameBack: nothing could
// be written while it failed, so both events are written once it works,
// each at its own time.
func TestStorageThatRecoversRecordsWhenItFailedAndWhenItCameBack(t *testing.T) {
	s := newServer(t, fakeBackend{})
	store := withAlerter(t, s)
	failed := time.Date(2026, 10, 9, 3, 0, 0, 0, time.Local)
	back := failed.Add(40 * time.Minute)
	s.storageTurned(history.Health{OK: true}, history.Health{Cause: "database or disk is full", Since: failed})
	if got := recorded(t, store); len(got) != 0 {
		t.Fatalf("recorded while failing: %+v", got)
	}
	s.storageTurned(history.Health{Cause: "database or disk is full", Since: failed}, history.Health{OK: true, Since: back})
	got := recorded(t, store)
	if len(got) != 2 ||
		got[0].Kind != "storage.unavailable" || !got[0].Time.Equal(failed) || got[0].Detail != `{"cause":"database or disk is full"}` ||
		got[1].Kind != "storage.available" || !got[1].Time.Equal(back) {
		t.Fatalf("events = %+v", got)
	}
}

// TestEventsWarnsWhileHistoryIsNotSaved: the page says so, and why, for as
// long as it lasts.
func TestEventsWarnsWhileHistoryIsNotSaved(t *testing.T) {
	s := newServer(t, fakeBackend{})
	store := withAlerter(t, s)
	if _, body := getSignedIn(t, s, eventsPath); strings.Contains(body, "History is not being saved") {
		t.Fatal("a warning while history is saved")
	}
	store.Fail("/data/verso cannot be written", time.Now())
	_, body := getSignedIn(t, s, eventsPath)
	if !strings.Contains(body, "History is not being saved.") || !strings.Contains(body, "/data/verso cannot be written") {
		t.Fatalf("no warning, or no cause, while history is not saved:\n%s", body)
	}
}

// TestStorageEventsReadAsSentences.
func TestStorageEventsReadAsSentences(t *testing.T) {
	identity := func(s string) string { return s }
	down := eventRow(history.Event{Kind: "storage.unavailable", Detail: `{"cause":"disk I/O error"}`}, time.Now(), identity)
	if got := down.Cells[2]; got.Text != "History could not be saved: disk I/O error" || got.Variant != "warning" {
		t.Errorf("unavailable = %+v", got)
	}
	up := eventRow(history.Event{Kind: "storage.available"}, time.Now(), identity)
	if got := up.Cells[2]; got.Text != "History is saved again" || got.Variant != "success" {
		t.Errorf("available = %+v", got)
	}
}

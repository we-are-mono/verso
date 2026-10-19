// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// sessionsAt is a session store on a clock the test moves.
func sessionsAt(t *testing.T) (*Sessions, *time.Time) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	return newSessionsClock(func() time.Time { return now }), &now
}

func saidOf(id int64) saidEvent {
	return saidEvent{ID: id, Event: history.Event{Kind: "acme.renewed"}}
}

// TestAnEventIsKeptForTheSessionsThatMaySeeIt: an event is kept for each live
// session whose grant answers yes, and for no other.
func TestAnEventIsKeptForTheSessionsThatMaySeeIt(t *testing.T) {
	s, _ := sessionsAt(t)
	allowed := s.CreateWithMetadata("sid-a", "root", "", "")
	refused := s.CreateWithMetadata("sid-b", "guest", "", "")
	s.tellAll(saidOf(1), func(sid string) bool { return sid == "sid-a" })
	if got := s.told(allowed, 0); len(got) != 1 || got[0].ID != 1 {
		t.Errorf("allowed session = %+v", got)
	}
	if got := s.told(refused, 0); len(got) != 0 {
		t.Errorf("refused session = %+v", got)
	}
}

// TestASessionKeepsItsLastMinuteOfEvents: at most eight, each for a minute
// from when it was recorded; reading leaves them; a reader that has an id
// gets only those after it.
func TestASessionKeepsItsLastMinuteOfEvents(t *testing.T) {
	s, now := sessionsAt(t)
	token := s.CreateWithMetadata("sid", "root", "", "")
	all := func(string) bool { return true }
	for id := int64(1); id <= 10; id++ {
		s.tellAll(saidOf(id), all)
	}
	got := s.told(token, 0)
	if len(got) != saidKept || got[0].ID != 3 || got[len(got)-1].ID != 10 {
		t.Fatalf("kept %+v, want the last %d", got, saidKept)
	}
	if again := s.told(token, 0); len(again) != saidKept {
		t.Error("reading took the events")
	}
	if after := s.told(token, 8); len(after) != 2 || after[0].ID != 9 {
		t.Errorf("after 8 = %+v", after)
	}
	*now = now.Add(saidFor + time.Second)
	s.tellAll(saidOf(11), all)
	if got := s.told(token, 0); len(got) != 1 || got[0].ID != 11 {
		t.Errorf("past their minute = %+v, want only the fresh one", got)
	}
}

// TestAnEventWakesEverySessionStream: every stream waiting on the session
// wakes, however many there are.
func TestAnEventWakesEverySessionStream(t *testing.T) {
	s, _ := sessionsAt(t)
	token := s.CreateWithMetadata("sid", "root", "", "")
	first, second := s.eventWake(token), s.eventWake(token)
	s.tellAll(saidOf(1), func(string) bool { return true })
	for i, wake := range []<-chan struct{}{first, second} {
		select {
		case <-wake:
		default:
			t.Errorf("stream %d was not woken", i+1)
		}
	}
	select {
	case <-s.eventWake(token):
		t.Error("a stream waiting after the event woke without one")
	default:
	}
}

// TestAnEndedSessionsEventsGoWithIt: a session signed out keeps nothing.
func TestAnEndedSessionsEventsGoWithIt(t *testing.T) {
	s, _ := sessionsAt(t)
	token := s.CreateWithMetadata("sid", "root", "", "")
	s.tellAll(saidOf(1), func(string) bool { return true })
	s.destroy(token)
	if got := s.told(token, 0); len(got) != 0 {
		t.Errorf("an ended session = %+v", got)
	}
	if len(s.said) != 0 {
		t.Errorf("kept for ended sessions: %v", s.said)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// Each session keeps the events said to it (ADR-028) for a minute, so the
// browser's one stream sends them as they are recorded, or as a page opens
// soon after. Nothing is taken on reading: the browser says each once, on a
// page someone is looking at, and an event sent to a page that was closing
// is sent again to the next.

// saidEvent is one event a session's browser says: its rowid, which the
// stream sends as its id, the event, and when the shell recorded it.
type saidEvent struct {
	ID       int64
	Event    history.Event
	Recorded time.Time
}

const (
	// saidKept is the most events a session keeps.
	saidKept = 8
	// saidFor is how long a session keeps an event: one said later is news
	// no more, and System → Events lists it.
	saidFor = time.Minute
)

// sessionEvents is one session's kept events, and the channel its streams
// wait on, closed and made anew as each event arrives so every one wakes.
type sessionEvents struct {
	kept []saidEvent
	wake chan struct{}
}

// tellAll keeps e for every live session may lets see it, asked outside the
// lock, since may asks rpcd, and wakes their streams.
func (s *Sessions) tellAll(e saidEvent, may func(sid string) bool) {
	type live struct{ token, sid string }
	s.mu.Lock()
	s.sweep(s.now())
	sessions := make([]live, 0, len(s.items))
	for token, sess := range s.items {
		sessions = append(sessions, live{token, sess.sid})
	}
	s.mu.Unlock()
	var allowed []string
	for _, l := range sessions {
		if may(l.sid) {
			allowed = append(allowed, l.token)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e.Recorded = s.now()
	s.forgetEnded()
	for _, token := range allowed {
		if _, ok := s.items[token]; !ok {
			continue
		}
		se := s.eventsOf(token)
		se.kept = append(se.kept, e)
		if len(se.kept) > saidKept {
			se.kept = se.kept[len(se.kept)-saidKept:]
		}
		close(se.wake)
		se.wake = make(chan struct{})
	}
}

// told is the session's kept events after the one numbered after, oldest
// first, each recorded within the last saidFor.
func (s *Sessions) told(token string, after int64) []saidEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[token]; !ok {
		return nil
	}
	se, ok := s.said[token]
	if !ok {
		return nil
	}
	fresh := s.now().Add(-saidFor)
	var out []saidEvent
	for _, e := range se.kept {
		if e.ID > after && !e.Recorded.Before(fresh) {
			out = append(out, e)
		}
	}
	return out
}

// eventWake is the channel that closes when the session is next told an
// event.
func (s *Sessions) eventWake(token string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventsOf(token).wake
}

// eventsOf is the token's kept events, made on first asking. The caller
// holds the lock.
func (s *Sessions) eventsOf(token string) *sessionEvents {
	if s.said == nil {
		s.said = map[string]*sessionEvents{}
	}
	se, ok := s.said[token]
	if !ok {
		se = &sessionEvents{wake: make(chan struct{})}
		s.said[token] = se
	}
	return se
}

// forgetEnded drops the kept events of sessions that have ended, waking
// their streams so each finds its session gone. The caller holds the lock.
func (s *Sessions) forgetEnded() {
	for token, se := range s.said {
		if _, ok := s.items[token]; !ok {
			close(se.wake)
			delete(s.said, token)
		}
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import "time"

// Health is whether the history is being saved: since when it has been, or
// since when and why it has not (ADR-019).
type Health struct {
	OK    bool
	Cause string
	Since time.Time
}

// Health is the store's health now.
func (s *Store) Health() Health {
	s.hmu.Lock()
	defer s.hmu.Unlock()
	return s.health
}

// OnHealth is told each time the health turns, with what it was before.
func (s *Store) OnHealth(f func(was, now Health)) {
	s.hmu.Lock()
	defer s.hmu.Unlock()
	s.onHealth = f
}

// Fail marks the history as not being saved for good, for a cause outside
// the file itself: no file could be opened, and the history is kept in
// memory, where every write succeeds without saving anything.
func (s *Store) Fail(cause string, at time.Time) {
	s.hmu.Lock()
	s.failed = true
	s.hmu.Unlock()
	s.turn(Health{Cause: cause, Since: at})
}

// note turns the health by how a write went: the first failure of a run, or
// the first success after one.
func (s *Store) note(err error, at time.Time) {
	s.hmu.Lock()
	failed := s.failed
	s.hmu.Unlock()
	switch {
	case failed:
	case err != nil:
		s.turn(Health{Cause: err.Error(), Since: at})
	default:
		s.turn(Health{OK: true, Since: at})
	}
}

func (s *Store) turn(next Health) {
	s.hmu.Lock()
	was, f := s.health, s.onHealth
	if was.OK == next.OK {
		s.hmu.Unlock()
		return
	}
	s.health = next
	s.hmu.Unlock()
	if f != nil {
		f(was, next)
	}
}

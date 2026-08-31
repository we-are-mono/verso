// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestSessionIdleExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	tok, _ := s.CreateWithMetadata("sid", "root", "", "")

	clk.advance(s.idle - time.Second) // just inside the idle window
	if _, ok := s.get(tok); !ok {
		t.Fatal("session should be live before the idle timeout")
	}
	clk.advance(s.idle + time.Second) // idle exceeded since the last get
	if _, ok := s.get(tok); ok {
		t.Error("session should expire after the idle timeout")
	}
}

func TestSessionAbsoluteExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	tok, _ := s.CreateWithMetadata("sid", "root", "", "")

	// Keep touching within the idle window; the absolute cap must still fire.
	expired := false
	for i := 0; i < 30; i++ {
		clk.advance(25 * time.Minute) // < 30m idle
		if _, ok := s.get(tok); !ok {
			expired = true
			break
		}
	}
	if !expired {
		t.Error("session should expire at the absolute cap despite continuous activity")
	}
}

// TestSessionExpiresAtNearerBound: the stated end of a session is whichever
// comes first — the idle window from its last use or the absolute cap from its
// creation. The page stamps this moment, so it must never overstate the session.
func TestSessionExpiresAtNearerBound(t *testing.T) {
	base := time.Unix(1_000_000, 0)
	s := newSessionsClock(func() time.Time { return base })

	fresh := session{created: base, lastSeen: base}
	if got, want := s.expiresAt(fresh), base.Add(s.idle); !got.Equal(want) {
		t.Errorf("fresh session expires at %v, want %v", got, want)
	}

	// Used continuously right up to the cap: the idle window would reach past it.
	late := session{created: base, lastSeen: base.Add(s.absolute - time.Minute)}
	if got, want := s.expiresAt(late), base.Add(s.absolute); !got.Equal(want) {
		t.Errorf("session near the cap expires at %v, want %v", got, want)
	}
}

// TestSessionAliveDoesNotSlideIdle: the liveness check a held-open stream makes
// leaves the idle clock alone — a browser polling in the background must not
// keep the operator signed in.
func TestSessionAliveDoesNotSlideIdle(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	tok, _ := s.CreateWithMetadata("sid", "root", "", "")

	clk.advance(s.idle - time.Minute)
	if !s.alive(tok) {
		t.Fatal("session should be live just inside the idle window")
	}
	clk.advance(2 * time.Minute) // past the window counted from creation
	if s.alive(tok) {
		t.Error("alive must not slide the idle window")
	}
}

func TestSessionSweepOnCreate(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	old, _ := s.CreateWithMetadata("a", "root", "", "")

	clk.advance(s.absolute + time.Hour) // old is now well past the absolute cap
	if _, err := s.CreateWithMetadata("b", "root", "", ""); err != nil {
		t.Fatalf("Create: %v", err) // Create sweeps expired entries
	}

	s.mu.Lock()
	n := len(s.items)
	s.mu.Unlock()
	if n != 1 {
		t.Errorf("sweep should have removed the expired session; have %d", n)
	}
	if _, ok := s.get(old); ok {
		t.Error("the expired session should be gone")
	}
}

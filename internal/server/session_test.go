// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time            { return c.t }
func (c *fakeClock) advance(d time.Duration)   { c.t = c.t.Add(d) }

func TestSessionIdleExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	tok, _ := s.Create("sid", "root")

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
	tok, _ := s.Create("sid", "root")

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

func TestSessionSweepOnCreate(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	old, _ := s.Create("a", "root")

	clk.advance(s.absolute + time.Hour) // old is now well past the absolute cap
	if _, err := s.Create("b", "root"); err != nil {
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

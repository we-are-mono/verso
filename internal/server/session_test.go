// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeKeeper records the sids it was asked to renew and destroy; alive is what
// Renew reports (a live sid, or one rpcd no longer knows).
type fakeKeeper struct {
	mu        sync.Mutex
	renewed   []string
	destroyed []string
	alive     bool
}

func (k *fakeKeeper) Renew(_ context.Context, sid string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.renewed = append(k.renewed, sid)
	return k.alive
}

func (k *fakeKeeper) Destroy(_ context.Context, sid string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.destroyed = append(k.destroyed, sid)
}

func (k *fakeKeeper) counts() (renewed, destroyed []string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.renewed...), append([]string(nil), k.destroyed...)
}

func TestSessionIdleExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	tok := s.CreateWithMetadata("sid", "root", "", "")

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
	tok := s.CreateWithMetadata("sid", "root", "", "")

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
	tok := s.CreateWithMetadata("sid", "root", "", "")

	clk.advance(s.idle - time.Minute)
	if !s.alive(tok) {
		t.Fatal("session should be live just inside the idle window")
	}
	clk.advance(2 * time.Minute) // past the window counted from creation
	if s.alive(tok) {
		t.Error("alive must not slide the idle window")
	}
}

// TestRenewerKeepsSIDWarmThenReleasesIt is the bug this whole change exists for:
// a session sits idle far longer than rpcd's 300 s session timeout, and its sid
// must stay usable — the renewer keeps it warm — until the Verso session itself
// ends on its own idle clock, at which point the sid is torn down (ADR-007 §7).
func TestRenewerKeepsSIDWarmThenReleasesIt(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	k := &fakeKeeper{alive: true}
	s.keeper = k
	tok := s.CreateWithMetadata("SID", "root", "", "")

	// Six minutes of inactivity — well past rpcd's 300 s — with the renewer
	// ticking. The old bug: the sid would be dead here. Now it is renewed.
	clk.advance(6 * time.Minute)
	s.renewTick(context.Background())
	if renewed, _ := k.counts(); len(renewed) != 1 || renewed[0] != "SID" {
		t.Fatalf("renewer should have kept SID warm, renewed=%v", renewed)
	}
	if !s.alive(tok) { // alive doesn't slide the idle clock — lastSeen stays at creation
		t.Fatal("session must survive past rpcd's 300 s window while renewed")
	}

	// Now let the Verso session's own idle window lapse. The sid is released.
	clk.advance(sessionIdleTimeout + time.Minute)
	s.renewTick(context.Background())
	if _, destroyed := k.counts(); len(destroyed) != 1 || destroyed[0] != "SID" {
		t.Errorf("an expired session's sid must be destroyed, destroyed=%v", destroyed)
	}
	if s.alive(tok) {
		t.Error("session should be gone after its idle window")
	}
}

// TestRenewerEvictsUnknownSID: when rpcd reports a live session's sid unknown
// (its own restart), the renewer signs that session out so the next request
// lands on the login page — not a shell whose every gated read fails.
func TestRenewerEvictsUnknownSID(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	k := &fakeKeeper{alive: false} // rpcd no longer knows this sid
	s.keeper = k
	tok := s.CreateWithMetadata("SID", "root", "", "")

	s.renewTick(context.Background())
	if s.alive(tok) {
		t.Error("a session whose sid rpcd forgot must be evicted")
	}
	if _, destroyed := k.counts(); len(destroyed) != 0 {
		t.Errorf("an already-dead sid must not be destroyed again, destroyed=%v", destroyed)
	}
}

// TestRenewerDestroysLoggedOutSID: an explicit logout queues the sid, and the
// next renewer pass tears it down rather than renewing it.
func TestRenewerDestroysLoggedOutSID(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	k := &fakeKeeper{alive: true}
	s.keeper = k
	tok := s.CreateWithMetadata("SID", "root", "", "")

	s.destroy(tok) // logout
	s.renewTick(context.Background())
	renewed, destroyed := k.counts()
	if len(destroyed) != 1 || destroyed[0] != "SID" {
		t.Errorf("a logged-out session's sid must be destroyed, destroyed=%v", destroyed)
	}
	if len(renewed) != 0 {
		t.Errorf("a logged-out session's sid must not be renewed, renewed=%v", renewed)
	}
}

// TestRenewerLifecycle: start/stop is idempotent and returns promptly, and a nil
// keeper starts nothing (a test or non-rpcd auth stays renewal-free).
func TestRenewerLifecycle(t *testing.T) {
	s := newSessionsClock(time.Now)
	s.startRenewer(nil, time.Minute) // nil keeper: no-op
	s.stopRenewer()                  // safe with no renewer running

	s2 := newSessionsClock(time.Now)
	s2.startRenewer(&fakeKeeper{alive: true}, time.Hour) // long interval: never ticks in the test
	s2.startRenewer(&fakeKeeper{alive: true}, time.Hour) // idempotent
	s2.stopRenewer()
	s2.stopRenewer() // idempotent
}

// TestRenewerStopBeforeStart: a stop before any start must not consume the
// ability to start one later — the earlier two-sync.Once design leaked an
// unstoppable goroutine here. stopRenewer blocks on the goroutine's exit, so if
// it returns, the renewer is provably stopped; a hang means the leak is back.
func TestRenewerStopBeforeStart(t *testing.T) {
	s := newSessionsClock(time.Now)
	s.stopRenewer() // no renewer yet — must be a clean no-op

	done := make(chan struct{})
	go func() {
		s.startRenewer(&fakeKeeper{alive: true}, time.Hour) // long interval: won't tick
		s.stopRenewer()                                     // must cancel and join the goroutine
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stopRenewer hung: a start after an early stop left an unstoppable renewer")
	}
}

func TestSessionSweepOnCreate(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newSessionsClock(clk.now)
	old := s.CreateWithMetadata("a", "root", "", "")

	clk.advance(s.absolute + time.Hour)       // old is now well past the absolute cap
	s.CreateWithMetadata("b", "root", "", "") // Create sweeps expired entries

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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"
)

func TestLoginLimiterLocksThenReleases(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newLoginLimiter(clk.now)

	for i := 0; i < loginMaxFailures; i++ {
		if !l.allowed("ip") {
			t.Fatalf("locked too early after %d failures", i)
		}
		l.fail("ip")
	}
	if l.allowed("ip") {
		t.Error("should be locked out after the failure limit")
	}
	clk.advance(loginLockout + time.Second)
	if !l.allowed("ip") {
		t.Error("should be allowed again once the lockout expires")
	}
}

// The sign-in page reads the standing count off the limiter, so it has to track
// the failures as they land — and go quiet at the lockout, where reaching the
// limit clears the counter it was counting toward.
func TestLoginLimiterCountsFailures(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newLoginLimiter(clk.now)

	if got := l.failures("ip"); got != 0 {
		t.Errorf("an address never seen has %d failures, want 0", got)
	}
	l.fail("ip")
	l.fail("ip")
	if got := l.failures("ip"); got != 2 {
		t.Errorf("failures = %d, want 2", got)
	}
	if got := l.failures("other"); got != 0 {
		t.Errorf("failures are per address; other = %d, want 0", got)
	}

	for i := l.failures("ip"); i < loginMaxFailures; i++ {
		l.fail("ip")
	}
	if got := l.failures("ip"); got != 0 {
		t.Errorf("the lockout clears the count, got %d", got)
	}
}

func TestLoginLimiterSuccessResets(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newLoginLimiter(clk.now)

	for i := 0; i < loginMaxFailures-1; i++ {
		l.fail("ip")
	}
	l.success("ip") // clears the near-lockout count
	for i := 0; i < loginMaxFailures-1; i++ {
		l.fail("ip")
	}
	if !l.allowed("ip") {
		t.Error("a success should reset the counter, so fewer-than-limit failures do not lock")
	}
}

func TestLoginLimiterIsPerKey(t *testing.T) {
	l := newLoginLimiter((&fakeClock{t: time.Unix(1_000_000, 0)}).now)
	for i := 0; i < loginMaxFailures; i++ {
		l.fail("attacker")
	}
	if l.allowed("attacker") {
		t.Error("attacker key should be locked")
	}
	if !l.allowed("victim") {
		t.Error("a different key must not be affected")
	}
}

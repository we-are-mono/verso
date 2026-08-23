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

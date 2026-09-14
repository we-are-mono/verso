// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	loginMaxFailures = 5
	loginLockout     = time.Minute
	// loginRetention bounds the attempts map: an entry untouched for this long
	// (an IP that failed a few times and never returned) is swept, so source-IP
	// rotation cannot grow the map without limit. Generous next to the lockout,
	// so an active brute-forcer is never swept out from under its own counter.
	loginRetention = 15 * time.Minute
)

type loginAttempt struct {
	fails    int
	lockedTo time.Time
	seen     time.Time // last activity, for retention sweeping
}

// loginLimiter throttles failed logins per client IP: after loginMaxFailures
// consecutive failures it locks that IP out for loginLockout (VS-06). A success
// clears the record. Per-IP (not per-account) on purpose — locking an account
// would let an attacker deny the admin access. The clock is injected so the
// lockout is testable without sleeping (ADR-003).
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt), now: now}
}

// allowed reports whether key may attempt a login now.
func (l *loginLimiter) allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.now().Before(l.attempts[key].lockedTo)
}

// fail records a failed attempt, locking the key out once it hits the limit.
func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a := l.attempts[key]
	a.fails++
	a.seen = now
	if a.fails >= loginMaxFailures {
		a.lockedTo = now.Add(loginLockout)
		a.fails = 0
	}
	l.attempts[key] = a
	// Sweep abandoned entries — cheap, and only on the failure path — so a run of
	// distinct source IPs each failing a few times can't grow the map unbounded.
	for k, v := range l.attempts {
		if now.Sub(v.seen) > loginRetention {
			delete(l.attempts, k)
		}
	}
}

// failures reports the consecutive failures standing against key. It is what
// the sign-in page states back to a visitor — someone has been guessing at this
// router, and that is worth knowing before you type anything. Zero once the key
// is locked out, since reaching the limit both locks and clears the count.
func (l *loginLimiter) failures(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts[key].fails
}

// success clears any failure record for the key.
func (l *loginLimiter) success(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// clientIP is the throttle key: the source address, port stripped. It uses
// RemoteAddr only — X-Forwarded-* is spoofable and not trusted here.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

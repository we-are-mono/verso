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
)

type loginAttempt struct {
	fails    int
	lockedTo time.Time
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
	a := l.attempts[key]
	a.fails++
	if a.fails >= loginMaxFailures {
		a.lockedTo = l.now().Add(loginLockout)
		a.fails = 0
	}
	l.attempts[key] = a
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

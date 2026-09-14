// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	sessionCookie          = "verso_session"
	sessionIdleTimeout     = 30 * time.Minute
	sessionAbsoluteTimeout = 12 * time.Hour
	// sessionRenewInterval is how often the renewer touches each live session's
	// rpcd sid. It must stay well inside rpcd's session timeout
	// (RPC_DEFAULT_SESSION_TIMEOUT, 300 s) so a sid never lapses mid-session
	// (ADR-007 §7); 60 s leaves a 5× margin against a missed tick.
	sessionRenewInterval = 60 * time.Second
)

// sidKeeper renews and tears down the rpcd session behind a Verso session. The
// rpcd sid runs on rpcd's own inactivity clock (300 s), far shorter than a Verso
// session, so it is kept warm while its session lives and destroyed when the
// session ends (ADR-007 §7). Renew reports whether rpcd still knows the sid, so a
// vanished session (rpcd restart) can be signed out rather than left half-working.
// Injected so the store stays testable without a real ubus.
type sidKeeper interface {
	Renew(ctx context.Context, sid string) (alive bool)
	Destroy(ctx context.Context, sid string)
}

// session is server-side state keyed by an opaque cookie token. The rpcd sid is
// held here and never sent to the browser — unlike LuCI, which exposes the sid
// because the browser calls ubus directly (FINDINGS F1).
type session struct {
	id       string // non-secret management handle; unlike the cookie token, safe in the sessions table
	sid      string
	username string
	csrf     string
	created  time.Time
	lastSeen time.Time
	address  string
	agent    string
	// flash is the one-shot confirmation carried across a POST→redirect→GET:
	// set by the action, shown by the next render, gone after (VS: server-side,
	// so nothing user-visible rides the URL).
	flash        string
	flashVariant string
}

// Sessions is the server's in-memory session store. A random opaque token maps
// to the held session; sessions expire on idle and at an absolute cap (VS-03).
// The clock is injected so expiry is testable without sleeping (ADR-003).
type Sessions struct {
	mu       sync.Mutex
	items    map[string]session
	now      func() time.Time
	idle     time.Duration
	absolute time.Duration
	// keeper renews and destroys the rpcd sid behind each session; nil disables
	// renewal (tests that need no ubus). pendingKill holds sids whose sessions
	// have ended, queued under the lock and drained by the renewer outside it.
	keeper      sidKeeper
	pendingKill []string
	// renewCancel and renewDone are the renewer goroutine's handle, guarded by mu:
	// non-nil renewCancel means a renewer is running. Both start (once, with a
	// keeper) and stop are idempotent, and stop before start is a safe no-op.
	renewCancel context.CancelFunc
	renewDone   chan struct{}
}

func newSessions() *Sessions { return newSessionsClock(time.Now) }

func newSessionsClock(now func() time.Time) *Sessions {
	return &Sessions{
		items:    make(map[string]session),
		now:      now,
		idle:     sessionIdleTimeout,
		absolute: sessionAbsoluteTimeout,
	}
}

// CreateWithMetadata mints a fresh opaque token for a session and returns it,
// opportunistically sweeping expired entries. It records the browser and peer
// address shown on System → Access; the separate management id can end a session
// without ever exposing its bearer cookie or rpcd sid to another browser.
func (s *Sessions) CreateWithMetadata(sid, username, address, agent string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", err
	}
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	s.mu.Lock()
	s.sweep(now)
	s.items[token] = session{id: id, sid: sid, username: username, csrf: csrf, created: now, lastSeen: now, address: address, agent: agent}
	s.mu.Unlock()
	return token, nil
}

// get returns a live session, sliding its idle window. An expired session is
// removed and reported absent.
func (s *Sessions) get(token string) (session, bool) {
	return s.lookup(token, true)
}

// peek authenticates a background refresh without counting it as activity.
func (s *Sessions) peek(token string) (session, bool) {
	return s.lookup(token, false)
}

func (s *Sessions) lookup(token string, touch bool) (session, bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	if !ok {
		return session{}, false
	}
	if s.expired(sess, now) {
		delete(s.items, token)
		s.killSID(sess.sid)
		return session{}, false
	}
	if touch {
		sess.lastSeen = now
		s.items[token] = sess
	}
	return sess, true
}

// alive reports whether a token still names a live session, leaving its idle
// clock untouched — the check a held-open stream makes on each tick. A browser
// keeping a connection open is not a person at the router, so the overview left
// on a screen signs out on inactivity like any other page; list() leaves the
// clocks alone for the same reason.
func (s *Sessions) alive(token string) bool {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	if !ok {
		return false
	}
	if s.expired(sess, now) {
		delete(s.items, token)
		s.killSID(sess.sid)
		return false
	}
	return true
}

// expiresAt is the moment a session ends if nothing touches it again: the idle
// window from its last use, or the absolute cap from its creation, whichever
// comes first. Every render states it, so it must never overstate the session.
func (s *Sessions) expiresAt(sess session) time.Time {
	idle := sess.lastSeen.Add(s.idle)
	if absolute := sess.created.Add(s.absolute); absolute.Before(idle) {
		return absolute
	}
	return idle
}

// SetFlash stores the session's one-shot confirmation for the next render.
func (s *Sessions) SetFlash(token, variant, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	if !ok {
		return
	}
	sess.flash, sess.flashVariant = message, variant
	s.items[token] = sess
}

// TakeFlash returns and clears the session's flash — each message shows once.
func (s *Sessions) TakeFlash(token string) (variant, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	if !ok || sess.flash == "" {
		return "", ""
	}
	variant, message = sess.flashVariant, sess.flash
	sess.flash, sess.flashVariant = "", ""
	s.items[token] = sess
	return variant, message
}

func (s *Sessions) destroy(token string) {
	s.mu.Lock()
	if sess, ok := s.items[token]; ok {
		delete(s.items, token)
		s.killSID(sess.sid)
	}
	s.mu.Unlock()
}

// list returns a point-in-time copy of the live sessions without touching their
// idle clocks. Merely viewing the table must not keep every browser signed in.
func (s *Sessions) list() []session {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	out := make([]session, 0, len(s.items))
	for _, sess := range s.items {
		out = append(out, sess)
	}
	return out
}

// destroyID removes the session carrying a non-secret management id.
func (s *Sessions) destroyID(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.items {
		if sess.id == id {
			delete(s.items, token)
			s.killSID(sess.sid)
			return true
		}
	}
	return false
}

func (s *Sessions) expired(sess session, now time.Time) bool {
	return now.Sub(sess.lastSeen) > s.idle || now.Sub(sess.created) > s.absolute
}

// sweep deletes expired sessions, queuing each one's sid for teardown. The
// caller holds the lock.
func (s *Sessions) sweep(now time.Time) {
	for token, sess := range s.items {
		if s.expired(sess, now) {
			delete(s.items, token)
			s.killSID(sess.sid)
		}
	}
}

// killSID queues a sid for the renewer to destroy in rpcd. The caller holds the
// lock. A no-op without a keeper (tests), so the queue never grows unbounded
// where nothing drains it.
func (s *Sessions) killSID(sid string) {
	if s.keeper != nil {
		s.pendingKill = append(s.pendingKill, sid)
	}
}

// startRenewer wires the sid keeper and starts the background renewer: one
// goroutine that keeps every live session's rpcd sid warm and tears down the sids
// of sessions that have ended (ADR-007 §7). Idempotent — a second call while one
// runs is a no-op; a nil keeper or a non-positive interval starts nothing,
// leaving the store renewal-free.
func (s *Sessions) startRenewer(keeper sidKeeper, interval time.Duration) {
	if keeper == nil || interval <= 0 {
		return
	}
	s.mu.Lock()
	if s.renewCancel != nil { // already running
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.keeper = keeper
	s.renewCancel = cancel
	s.renewDone = make(chan struct{})
	s.mu.Unlock()
	go s.renewLoop(ctx, interval)
}

// stopRenewer halts the renewer and waits for it to exit. Idempotent, and a safe
// no-op when no renewer is running (including a stop before any start) — it never
// consumes the ability to start one later.
func (s *Sessions) stopRenewer() {
	s.mu.Lock()
	cancel, done := s.renewCancel, s.renewDone
	s.renewCancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (s *Sessions) renewLoop(ctx context.Context, interval time.Duration) {
	defer close(s.renewDone)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.renewTick(ctx)
		}
	}
}

// renewTick is one renewal pass: destroy the sids of ended sessions, then renew
// the sids of live ones — evicting any session whose sid rpcd no longer knows, so
// its browser is sent to sign in again rather than shown a half-working shell.
// Keeper I/O runs outside the store lock. It is the unit the timer drives, and
// the seam tests call directly for a deterministic pass.
func (s *Sessions) renewTick(ctx context.Context) {
	live, dead := s.collect()
	for _, sid := range dead {
		s.keeper.Destroy(ctx, sid)
	}
	for _, sid := range live {
		if !s.keeper.Renew(ctx, sid) {
			s.evictBySID(sid)
		}
	}
}

// collect sweeps expired sessions (queuing their sids to destroy), then returns
// the live sids to renew and the queued dead sids. It slides no idle clock:
// renewal keeps a session's sid ready, it does not keep the session alive.
func (s *Sessions) collect() (live, dead []string) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	for _, sess := range s.items {
		live = append(live, sess.sid)
	}
	dead = s.pendingKill
	s.pendingKill = nil
	return live, dead
}

// evictBySID drops every session carrying sid — the response to rpcd reporting
// that sid unknown. The sid is already gone in rpcd, so nothing is queued to
// destroy; the browser's next request lands on the login page.
func (s *Sessions) evictBySID(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.items {
		if sess.sid == sid {
			delete(s.items, token)
		}
	}
}

// randomToken returns 256 bits of hex-encoded entropy — the opaque cookie value.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

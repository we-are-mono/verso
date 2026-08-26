// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	sessionCookie          = "verso_session"
	sessionIdleTimeout     = 30 * time.Minute
	sessionAbsoluteTimeout = 12 * time.Hour
)

// session is server-side state keyed by an opaque cookie token. The rpcd sid is
// held here and never sent to the browser — unlike LuCI, which exposes the sid
// because the browser calls ubus directly (FINDINGS F1).
type session struct {
	sid      string
	username string
	csrf     string
	created  time.Time
	lastSeen time.Time
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

// Create mints a fresh opaque token for a session and returns it, opportunistically
// sweeping expired entries.
func (s *Sessions) Create(sid, username string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	s.mu.Lock()
	s.sweep(now)
	s.items[token] = session{sid: sid, username: username, csrf: csrf, created: now, lastSeen: now}
	s.mu.Unlock()
	return token, nil
}

// get returns a live session, sliding its idle window. An expired session is
// removed and reported absent.
func (s *Sessions) get(token string) (session, bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	if !ok {
		return session{}, false
	}
	if s.expired(sess, now) {
		delete(s.items, token)
		return session{}, false
	}
	sess.lastSeen = now
	s.items[token] = sess
	return sess, true
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
	delete(s.items, token)
	s.mu.Unlock()
}

func (s *Sessions) expired(sess session, now time.Time) bool {
	return now.Sub(sess.lastSeen) > s.idle || now.Sub(sess.created) > s.absolute
}

// sweep deletes expired sessions. The caller holds the lock.
func (s *Sessions) sweep(now time.Time) {
	for token, sess := range s.items {
		if s.expired(sess, now) {
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

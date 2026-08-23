// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

const sessionCookie = "verso_session"

// session is server-side state keyed by an opaque cookie token. The rpcd sid is
// held here and never sent to the browser — unlike LuCI, which exposes the sid
// because the browser calls ubus directly (FINDINGS F1).
type session struct {
	sid      string
	username string
}

// Sessions is the server's in-memory session store. It is the shell's own state
// (not an injected dependency): a random opaque token maps to the held session.
type Sessions struct {
	mu    sync.Mutex
	items map[string]session
}

func newSessions() *Sessions { return &Sessions{items: make(map[string]session)} }

// Create mints a fresh opaque token for a session and returns it.
func (s *Sessions) Create(sid, username string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.items[token] = session{sid: sid, username: username}
	s.mu.Unlock()
	return token, nil
}

func (s *Sessions) get(token string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[token]
	return sess, ok
}

func (s *Sessions) destroy(token string) {
	s.mu.Lock()
	delete(s.items, token)
	s.mu.Unlock()
}

// randomToken returns 256 bits of hex-encoded entropy — the opaque cookie value.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"time"
)

// keptSession is one session as it is written out: every field the store
// holds, named, because the in-memory struct keeps them unexported.
type keptSession struct {
	Token        string    `json:"token"`
	ID           string    `json:"id"`
	SID          string    `json:"sid"`
	Username     string    `json:"username"`
	CSRF         string    `json:"csrf"`
	Created      time.Time `json:"created"`
	LastSeen     time.Time `json:"lastSeen"`
	Address      string    `json:"address"`
	Agent        string    `json:"agent"`
	Flash        string    `json:"flash,omitempty"`
	FlashVariant string    `json:"flashVariant,omitempty"`
}

// keep writes the live sessions to path, readable by the shell alone: they are
// bearer tokens. It is how a dev shell hands its signed-in browsers to the one
// that replaces it (Server.Close); a device never writes them.
func (s *Sessions) keep(path string) error {
	now := s.now()
	s.mu.Lock()
	s.sweep(now)
	kept := make([]keptSession, 0, len(s.items))
	for token, sess := range s.items {
		kept = append(kept, keptSession{
			Token: token, ID: sess.id, SID: sess.sid, Username: sess.username, CSRF: sess.csrf,
			Created: sess.created, LastSeen: sess.lastSeen, Address: sess.address, Agent: sess.agent,
			Flash: sess.flash, FlashVariant: sess.flashVariant,
		})
	}
	s.mu.Unlock()
	body, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

// restoreKeptSessions takes back what the shell before this one kept. A store
// it cannot read signs those browsers out, which is where they would be
// without it; it is said, not fatal.
func (s *Server) restoreKeptSessions() {
	if err := s.sessions.restore(s.keptSessions); err != nil {
		log.Printf("verso: restore kept sessions: %v", err)
	}
}

// restore reads back what keep wrote and removes the file, so a store is
// restored once. The clocks ran while the shell was down, so a session that
// ended in the meantime stays ended; the renewer's next tick signs out any
// whose rpcd sid did not survive either. An absent file is the ordinary first
// start.
func (s *Sessions) restore(path string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	var kept []keptSession
	if err := json.Unmarshal(body, &kept); err != nil {
		return err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range kept {
		sess := session{
			id: k.ID, sid: k.SID, username: k.Username, csrf: k.CSRF,
			created: k.Created, lastSeen: k.LastSeen, address: k.Address, agent: k.Agent,
			flash: k.Flash, flashVariant: k.FlashVariant,
		}
		if k.Token == "" || s.expired(sess, now) {
			continue
		}
		s.items[k.Token] = sess
	}
	return nil
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// rollbackGrace is how long past the rollback window an apply is given
// before it is taken as reverted: the device reverts at the window's end, and
// a confirm on its way in should not be beaten to it.
const rollbackGrace = 5 * time.Second

// applyWatch is the applies awaiting their confirm, one per session's stage
// (ADR-010): kept, it is recorded as applied; its window passing first, the
// router reverted it, and it is recorded as rolled back (ADR-019).
type applyWatch struct {
	mu  sync.Mutex
	seq int
	by  map[string]pendingApply
}

type pendingApply struct {
	seq   int
	pages []string
}

// stagedPages is the pages sid's stage holds changes from, by their English
// names; the Events page says them in the reader's language.
func (s *Server) stagedPages(ctx context.Context, sid string) []string {
	identity := func(s string) string { return s }
	view := s.staged(ctx, sid, identity, func(string) func(string) string { return identity })
	pages := make([]string, 0, len(view.Groups))
	for _, g := range view.Groups {
		pages = append(pages, g.Page)
	}
	return pages
}

// watchApply waits for sid's apply of pages to be confirmed, and records it
// rolled back if its window passes first.
func (s *Server) watchApply(sid string, pages []string) {
	if s.historyStore == nil {
		return
	}
	s.applies.mu.Lock()
	s.applies.seq++
	seq := s.applies.seq
	if s.applies.by == nil {
		s.applies.by = map[string]pendingApply{}
	}
	s.applies.by[sid] = pendingApply{seq: seq, pages: pages}
	s.applies.mu.Unlock()
	s.afterRollback(time.Duration(uciRollbackTimeout)*time.Second+rollbackGrace, func() {
		if pages, ok := s.takeApply(sid, seq); ok {
			s.recordApply("changes.rolledback", pages)
		}
	})
}

// confirmedApply records sid's apply as kept.
func (s *Server) confirmedApply(sid string) {
	s.applies.mu.Lock()
	pending, ok := s.applies.by[sid]
	s.applies.mu.Unlock()
	if ok {
		if pages, taken := s.takeApply(sid, pending.seq); taken {
			s.recordApply("changes.applied", pages)
		}
	}
}

// takeApply claims sid's pending apply numbered seq, once.
func (s *Server) takeApply(sid string, seq int) ([]string, bool) {
	s.applies.mu.Lock()
	defer s.applies.mu.Unlock()
	pending, ok := s.applies.by[sid]
	if !ok || pending.seq != seq {
		return nil, false
	}
	delete(s.applies.by, sid)
	return pending.pages, true
}

func (s *Server) recordApply(kind string, pages []string) {
	detail, _ := json.Marshal(struct {
		Pages []string `json:"pages"`
	}{pages})
	if err := s.historyStore.Record(context.Background(), history.Event{Time: time.Now(), Kind: kind, Detail: string(detail)}); err != nil {
		log.Printf("verso: event not recorded: %v", err)
	}
}

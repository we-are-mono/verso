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

// pendingApply is an apply awaiting its confirm: the stage it applied, as
// config history keeps it (stagedChange), read before the apply emptied it,
// and the entries it undoes once kept.
type pendingApply struct {
	seq      int
	change   history.Change
	reverses []int64
}

// watchApply waits for sid's apply of change to be confirmed, and records it
// rolled back if its window passes first. Kept, it undoes reverses.
func (s *Server) watchApply(sid string, change history.Change, reverses []int64) {
	if s.historyStore == nil && s.changeLog == nil {
		return
	}
	s.applies.mu.Lock()
	s.applies.seq++
	seq := s.applies.seq
	if s.applies.by == nil {
		s.applies.by = map[string]pendingApply{}
	}
	s.applies.by[sid] = pendingApply{seq: seq, change: change, reverses: reverses}
	s.applies.mu.Unlock()
	s.afterRollback(time.Duration(uciRollbackTimeout)*time.Second+rollbackGrace, func() {
		if pending, ok := s.takeApply(sid, seq); ok {
			s.recordApply(history.ChangeRolledBack, pending.change, nil)
		}
	})
}

// confirmedApply records sid's apply as kept.
func (s *Server) confirmedApply(sid string) {
	s.applies.mu.Lock()
	pending, ok := s.applies.by[sid]
	s.applies.mu.Unlock()
	if ok {
		if pending, taken := s.takeApply(sid, pending.seq); taken {
			s.recordApply(history.ChangeApplied, pending.change, pending.reverses)
		}
	}
}

// takeApply claims sid's pending apply numbered seq, once.
func (s *Server) takeApply(sid string, seq int) (pendingApply, bool) {
	s.applies.mu.Lock()
	defer s.applies.mu.Unlock()
	pending, ok := s.applies.by[sid]
	if !ok || pending.seq != seq {
		return pendingApply{}, false
	}
	delete(s.applies.by, sid)
	return pending, true
}

// changesDetail is an apply's event detail: the pages its changes were made
// on, and its entry in config history, 0 where it was not kept.
type changesDetail struct {
	Pages  []string `json:"pages"`
	Change int64    `json:"change,omitempty"`
}

// recordApply keeps change as applied or rolled back: in config history
// (ADR-021), with the entries it undoes marked so, and as the event naming
// its pages and opening its entry (ADR-019).
func (s *Server) recordApply(status string, change history.Change, reverses []int64) {
	now := time.Now()
	var id int64
	if s.changeLog != nil {
		change.Time, change.Status = now, status
		var err error
		if id, err = s.changeLog.RecordChange(context.Background(), change); err != nil {
			log.Printf("verso: change not recorded: %v", err)
		} else if len(reverses) > 0 {
			if err := s.changeLog.MarkUndone(context.Background(), reverses, id); err != nil {
				log.Printf("verso: changes not marked undone: %v", err)
			}
		}
	}
	if s.historyStore == nil {
		return
	}
	kind := "changes.applied"
	if status == history.ChangeRolledBack {
		kind = "changes.rolledback"
	}
	detail, _ := json.Marshal(changesDetail{Pages: changePages(change), Change: id})
	if err := s.historyStore.Record(context.Background(), history.Event{Time: now, Kind: kind, Detail: string(detail)}); err != nil {
		log.Printf("verso: event not recorded: %v", err)
	}
}

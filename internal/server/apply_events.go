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
// the entries it undoes once kept, and when it was made.
type pendingApply struct {
	seq      int
	change   history.Change
	reverses []int64
	started  time.Time
	restage  applyRestage
}

// watchApply waits for sid's apply of change to be confirmed, and records it
// rolled back if its window passes first: the router has put its settings
// back, so what the apply carried is staged again as it was (restage). Kept,
// it undoes reverses.
func (s *Server) watchApply(sid string, change history.Change, reverses []int64, restage applyRestage) {
	s.applies.mu.Lock()
	s.applies.seq++
	seq := s.applies.seq
	if s.applies.by == nil {
		s.applies.by = map[string]pendingApply{}
	}
	s.applies.by[sid] = pendingApply{seq: seq, change: change, reverses: reverses, started: time.Now(), restage: restage}
	s.applies.mu.Unlock()
	s.afterRollback(time.Duration(uciRollbackTimeout)*time.Second+rollbackGrace, func() {
		if pending, ok := s.takeApply(sid, seq); ok {
			s.recordApply(history.ChangeRolledBack, pending)
			s.stageAgain(sid, pending.restage)
			s.stageEnds.set(sid, stageEnd{Kind: stageRolledBack, At: time.Now(), Count: pending.restage.count, Back: pending.restage.back})
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
			s.recordApply(history.ChangeApplied, pending)
			s.stageEnds.set(sid, stageEnd{Kind: stageApplied, At: time.Now(), Count: pending.restage.count, Back: pending.restage.back})
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
func (s *Server) recordApply(status string, pending pendingApply) {
	now := time.Now()
	change := pending.change
	// What the apply wrote, and what a rollback wrote back, are its own, not
	// edits made outside Verso.
	s.expected.expect(changedConfigs(change), pending.started, now.Add(ownWriteSlack))
	var id int64
	if s.changeLog != nil {
		change.Time, change.Status = now, status
		var err error
		if id, err = s.changeLog.RecordChange(context.Background(), change); err != nil {
			log.Printf("verso: change not recorded: %v", err)
		} else if status == history.ChangeApplied && len(pending.reverses) > 0 {
			if err := s.changeLog.MarkUndone(context.Background(), pending.reverses, id); err != nil {
				log.Printf("verso: changes not marked undone: %v", err)
			}
		}
		s.foldHistory(context.Background())
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

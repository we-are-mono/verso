// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"
)

// How a session's stage last ended: applied and kept, applied and put back
// by the router, or let go of.
const (
	stageApplied    = "applied"
	stageRolledBack = "rolledback"
	stageDiscarded  = "discarded"
)

// stageEnd is how a session's stage last ended, what the Pending changes
// page says once it holds nothing more (or, after a rollback, the callout
// over what is pending again): when, how many changes, and the page the most
// recent of them was made on, its way back.
type stageEnd struct {
	Kind  string
	At    time.Time
	Count int
	// Back is who staged the most recent change, where the page leads back
	// to; its Href is empty where no page is on record.
	Back stageAuthor
}

// stageEnds is each session's last stage end.
type stageEnds struct {
	mu sync.Mutex
	by map[string]stageEnd
}

func (e *stageEnds) set(sid string, end stageEnd) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.by == nil {
		e.by = map[string]stageEnd{}
	}
	e.by[sid] = end
}

func (e *stageEnds) of(sid string) (stageEnd, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	end, ok := e.by[sid]
	return end, ok
}

// latestAuthor is who staged the most recent row of view, where any row's
// time is on record.
func latestAuthor(view stagedView) stageAuthor {
	var latest stageAuthor
	for _, g := range view.Groups {
		for _, item := range g.Items {
			if item.authored && item.author.At.After(latest.At) {
				latest = item.author
			}
		}
	}
	return latest
}

// applyRestage is what an apply would stage again should the router put its
// settings back: the stage's changes as rpcd journals them, a secret's value
// with them (held in memory for the window, never kept), who staged each,
// and each reordered config's staged order.
type applyRestage struct {
	ops    map[string][]replayOp
	orders map[string][]string
	count  int
	back   stageAuthor
}

// restageOf reads what an apply of view would stage again, before the apply
// empties the stage.
func (s *Server) restageOf(ctx context.Context, sid string, view stagedView) applyRestage {
	re := applyRestage{ops: map[string][]replayOp{}, orders: map[string][]string{}, count: view.Count, back: latestAuthor(view)}
	journal, err := s.backend.UCIChanges(ctx, sid)
	if err != nil {
		log.Printf("verso: what a rollback would stage again could not be read: %v", err)
		return re
	}
	declared := s.declaredConfigsUnion()
	for config, list := range journal {
		if !declared[config] {
			continue
		}
		for _, ch := range coalesceChanges(list) {
			if ch[0] == "file" {
				continue
			}
			re.ops[config] = append(re.ops[config], s.recorded(sid, config, ch))
		}
		if order := s.stagedOrder(ctx, sid, config, re.ops[config]); order != nil {
			re.orders[config] = order
		}
	}
	return re
}

// stageAgain stages what a rolled-back apply carried, config by config, as it
// was. A session rpcd no longer knows cannot be staged into; what fails is
// logged, and config history keeps the apply's "Stage this change again".
func (s *Server) stageAgain(sid string, re applyRestage) {
	configs := make([]string, 0, len(re.ops))
	for config := range re.ops {
		configs = append(configs, config)
	}
	slices.Sort(configs)
	for _, config := range configs {
		if err := s.replay(context.Background(), sid, config, re.ops[config], re.orders[config]); err != nil {
			log.Printf("verso: a rolled-back apply's %s could not be staged again: %v", config, err)
		}
	}
}

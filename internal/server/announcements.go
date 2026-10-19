// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
)

// What root announces (ADR-028): `verso announce` leaves each event in the
// events spool, and the shell takes them every spoolEvery, oldest first,
// records each in the history once, and lets it go.

// SetPluginsDir names where the plugins' manifests and registries are, so an
// event a plugin's upgrade declares is read from its registry before the
// rescan its package asks for.
func (s *Server) SetPluginsDir(dir string) { s.pluginsDir = dir }

// watchAnnouncements takes what root announces until ctx ends. It runs while
// an apply waits for its confirm, as config readings do not: an event is no
// reading of config.
func (s *Server) watchAnnouncements(ctx context.Context, store *history.Store, dir string) {
	tick := time.NewTicker(spoolEvery)
	defer tick.Stop()
	for {
		s.takeAnnouncements(ctx, store, dir)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// takeAnnouncements records each announcement waiting in dir, oldest first,
// and lets it go. One that is not root's, not of a kind root may announce, or
// not of the format this shell reads is let go unrecorded, why in the log;
// one the history cannot take now stays, with those after it, for the next
// pass.
func (s *Server) takeAnnouncements(ctx context.Context, store *history.Store, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	kinds := s.eventKinds()
	for _, entry := range entries {
		name := entry.Name()
		if !events.IsSpoolName(name) {
			continue
		}
		path := filepath.Join(dir, name)
		a, err := events.ReadAnnouncement(dir, name, s.announcerUID)
		if err != nil {
			log.Printf("verso: announcement %s let go: %v", name, err)
			_ = os.Remove(path)
			continue
		}
		e, k, ok := kinds.Record(a, name, s.pluginsDir)
		if !ok {
			log.Printf("verso: announcement %s let go: %s is no kind root may announce", name, a.Kind)
			_ = os.Remove(path)
			continue
		}
		id, err := store.RecordOnce(ctx, e, name)
		if err != nil {
			return
		}
		_ = os.Remove(path)
		if id != 0 {
			s.sayEvent(ctx, id, e, k)
		}
	}
}

// sayEvent keeps e, recorded as id, for every signed-in session rpcd lets see
// it, when its kind is said: their browsers' streams say it as the top-right
// notification (ADR-028). The router's own kinds are never said this way.
func (s *Server) sayEvent(ctx context.Context, id int64, e history.Event, k events.Kind) {
	if !k.Said {
		return
	}
	probe, known := kindProbe(k)
	if !known {
		return
	}
	s.sessions.tellAll(saidEvent{ID: id, Event: e}, func(sid string) bool {
		return probe == nil || s.grantsFor(sid).may(ctx, *probe)
	})
}

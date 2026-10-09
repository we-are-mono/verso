// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"log"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/history"
)

// storageTurned answers the history starting or stopping being saved
// (ADR-019). While it fails nothing can be written to it, so the alert goes
// out directly and both events are written once it works again, each at its
// own time.
func (s *Server) storageTurned(was, now history.Health) {
	ctx := context.Background()
	if !now.OK {
		if s.alerter != nil {
			s.alerter.Urgent(ctx, alerts.KindStorage, "history is not being saved",
				now.Cause+". Measurements and events are kept in memory until the storage answers again.")
		}
		return
	}
	cause, _ := json.Marshal(map[string]string{"cause": was.Cause})
	for _, e := range []history.Event{
		{Time: was.Since, Kind: "storage.unavailable", Detail: string(cause)},
		{Time: now.Since, Kind: "storage.available"},
	} {
		if err := s.historyStore.Record(ctx, e); err != nil {
			log.Printf("verso: event not recorded: %v", err)
		}
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"sync"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// wanMemo caches one WANStatus read for the lifetime of a single request. The
// overview page and the sidebar both state the uplink's live posture, and both
// would otherwise dial the backend on their own — two reads that could disagree
// and one round trip wasted. The memo makes the read happen at most once and
// hands the same answer to every reader on the request.
type wanMemo struct {
	once  sync.Once
	state openwrt.WANState
	err   error
}

type wanMemoKeyType struct{}

var wanMemoKey wanMemoKeyType

// withWANMemo attaches a fresh, unfilled memo to the request context — done once
// per authenticated request, so every reader downstream shares the same read.
func withWANMemo(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), wanMemoKey, &wanMemo{}))
}

// wanStatus returns the request's WAN posture, reading the backend at most once
// per request. Without a memo in context (a code path that never passed through
// requireAuth), it falls back to a direct read so callers never depend on the
// middleware having run.
func (s *Server) wanStatus(r *http.Request) (openwrt.WANState, error) {
	memo, ok := r.Context().Value(wanMemoKey).(*wanMemo)
	if !ok {
		return s.backend.WANStatus(r.Context(), s.sessionSID(r))
	}
	memo.once.Do(func() {
		memo.state, memo.err = s.backend.WANStatus(r.Context(), s.sessionSID(r))
	})
	return memo.state, memo.err
}

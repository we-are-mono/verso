// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"sync"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// requestMemo caches one backend read for the lifetime of a single request.
// Several parts of one page state the same live fact, and each would otherwise
// dial the backend on its own — reads that could disagree, and ubus round trips
// that are the router's CPU, the budget a LAN page spends. The memo makes the
// read happen at most once and hands the same answer to every reader on the
// request.
type requestMemo[T any] struct {
	once  sync.Once
	value T
	err   error
}

// memoized returns read's answer, reading at most once per request where a memo
// is installed under key. Without one (a code path that never installed it) it
// reads directly, so callers never depend on where the memo is installed.
func memoized[T any](ctx context.Context, key any, read func() (T, error)) (T, error) {
	memo, ok := ctx.Value(key).(*requestMemo[T])
	if !ok {
		return read()
	}
	memo.once.Do(func() { memo.value, memo.err = read() })
	return memo.value, memo.err
}

type wanMemoKey struct{}

// withWANMemo attaches a fresh, unfilled WAN-posture memo to the request
// context — done once per authenticated request, so the overview page and the
// sidebar share one WANStatus read.
func withWANMemo(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), wanMemoKey{}, &requestMemo[openwrt.WANState]{}))
}

// wanStatus returns the request's WAN posture, read at most once per request.
func (s *Server) wanStatus(r *http.Request) (openwrt.WANState, error) {
	return memoized(r.Context(), wanMemoKey{}, func() (openwrt.WANState, error) {
		return s.backend.WANStatus(r.Context(), s.sessionSID(r))
	})
}

type stageMemoKey struct{}

// withStageMemo gives a request's context one shared read of the stage. A page
// asks the stage twice — for the marks on the controls whose options wait, and
// for the chip — and both must read the same stage. The memo is installed where
// a page is answered, after anything that request stages has been written.
func withStageMemo(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), stageMemoKey{}, &requestMemo[map[string][][]string]{}))
}

// stageChanges reads the pending changes, once per request where a memo is
// installed.
func (s *Server) stageChanges(ctx context.Context, sid string) (map[string][][]string, error) {
	return memoized(ctx, stageMemoKey{}, func() (map[string][][]string, error) {
		return s.backend.UCIChanges(ctx, sid)
	})
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/we-are-mono/verso/internal/listen"
)

// An apply that changes verso.web changes the listeners of the shell serving
// it (ADR-017 §1): the staged ones are bound beside the open ones before the
// router is asked, the page moves to them when its own listener goes, and the
// apply's confirm keeps them while its rollback drops them.

// uciListenerRollbackTimeout is the window an apply that changes the listeners
// holds: a browser meeting a new port may stop at a certificate warning first.
const uciListenerRollbackTimeout = 90

// stringList reads a uci option that may be a list or a single value.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

// stagedListeners is the listeners the session's stage sets in verso.web,
// and whether they differ from those the shell answers on. A stage that
// leaves verso untouched, or sets what is open, changes nothing; one that
// sets no listener beside LuCI leaves the shell beside it.
func (s *Server) stagedListeners(r *http.Request) (listen.Config, bool, error) {
	if s.listeners == nil {
		return listen.Config{}, false, nil
	}
	ctx, sid := r.Context(), s.sessionSID(r)
	changes, err := s.backend.UCIChanges(ctx, sid)
	if err != nil || len(changes[versoConfig]) == 0 {
		return listen.Config{}, false, nil
	}
	snapshot, err := s.backend.UCIConfig(ctx, sid, versoConfig)
	if err != nil {
		return listen.Config{}, false, fmt.Errorf("read %s config: %w", versoConfig, err)
	}
	web := asSection(snapshot["web"])
	redirect, _ := web["redirect_https"].(string)
	staged, err := listen.New(stringList(web["listen_https"]), stringList(web["listen_http"]), redirect)
	if err != nil {
		return listen.Config{}, false, err
	}
	now := s.listeners.Current()
	if staged.Defaulted && now.IsBeside {
		return staged, false, nil
	}
	same := slices.Equal(staged.HTTPS, now.HTTPS) && slices.Equal(staged.HTTP, now.HTTP) && staged.Redirect == now.Redirect
	return staged, !same, nil
}

// servedHere reports whether c answers the page where it was reached: a
// listener of its scheme on the port it came in on, at its address or every
// address. On HTTP with the redirect on, the page's own requests would be
// sent away, so it is not.
func servedHere(c listen.Config, r *http.Request) bool {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || local == nil {
		return true
	}
	host, port, err := net.SplitHostPort(local.String())
	if err != nil {
		return true
	}
	addrs := c.HTTPS
	if r.TLS == nil {
		if c.Redirect {
			return false
		}
		addrs = c.HTTP
	}
	for _, addr := range addrs {
		h, p, err := net.SplitHostPort(addr)
		if err == nil && p == port && (h == "" || h == "0.0.0.0" || h == "::" || h == host) {
			return true
		}
	}
	return false
}

// moveTo is where the page goes once the apply is made: the first HTTPS
// listener c names, at the host this browser reached the router by, or "" when
// c still answers it here.
func moveTo(c listen.Config, r *http.Request) string {
	if servedHere(c, r) || len(c.HTTPS) == 0 {
		return ""
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	_, port, _ := net.SplitHostPort(c.HTTPS[0])
	if port == "443" {
		if net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil {
			return "https://[" + host + "]"
		}
		return "https://" + host
	}
	return "https://" + net.JoinHostPort(host, port)
}

// awaitListenerConfirm drops the staged listeners once the apply's window has
// passed unconfirmed, as rpcd restores the configuration they came from.
func (s *Server) awaitListenerConfirm(window int) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	s.listenerPending = true
	s.afterRollback(time.Duration(window)*time.Second, func() {
		s.listenerMu.Lock()
		pending := s.listenerPending
		s.listenerPending = false
		s.listenerMu.Unlock()
		if pending {
			s.listeners.Drop()
		}
	})
}

// keepListeners keeps the staged listeners of a confirmed apply, if it staged
// any.
func (s *Server) keepListeners() {
	s.listenerMu.Lock()
	pending := s.listenerPending
	s.listenerPending = false
	s.listenerMu.Unlock()
	if pending {
		s.listeners.Keep()
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// securityHeaders sets defensive response headers on every reply (VS-07). The
// shell ships no JavaScript, so the CSP forbids scripts outright; styles are the
// embedded stylesheet (inline), and images allow data: and https: for the raw
// bridge. frame-ancestors + X-Frame-Options block clickjacking of a root panel.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'none'; style-src 'unsafe-inline'; " +
		"img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func hostSet(hosts []string) map[string]bool {
	m := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			m[h] = true
		}
	}
	return m
}

// hostGuard rejects requests whose Host is not allow-listed, and cross-origin
// state-changing requests (VS-01: DNS-rebinding and cross-site write defense).
// An empty allowlist disables the check — tests run that way; production always
// configures one (see main).
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			http.Error(w, "bad host", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && !s.originAllowed(o) {
				http.Error(w, "cross-origin request blocked", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(host string) bool {
	if len(s.allowedHosts) == 0 {
		return true
	}
	return s.allowedHosts[strings.ToLower(bareHost(host))]
}

func (s *Server) originAllowed(origin string) bool {
	if len(s.allowedHosts) == 0 {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return s.allowedHosts[strings.ToLower(bareHost(u.Host))]
}

func bareHost(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/datatype"
)

// securityHeaders sets defensive response headers on every reply (VS-07). The
// shell ships its own first-party JS (htmx + Alpine + verso.js, ADR-004), so
// script-src is 'self' — no 'unsafe-inline' and no 'unsafe-eval' (Alpine's CSP
// build needs neither), which keeps injected or plugin-supplied markup unable to
// execute: html/template escaping blocks tag injection, the CSP blocks inline and
// external scripts. Styles are the embedded stylesheet (inline); images allow
// data: and https: for the raw bridge. frame-ancestors + X-Frame-Options block
// clickjacking of a root panel.
//
// A form may also land on this router over HTTPS on another port: moving the
// web interface's ports answers its form there (ADR-017 §1). Only a Host that
// is a plain name or IPv4 address is written into the policy.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'; " +
		"img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		policy := csp
		if host := plainHost(r.Host); host != "" {
			policy += " https://" + host + ":*"
		}
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		// Verso is a local application, not a website: every response is live
		// state served over the LAN, where a fetch is effectively free. Nothing
		// is ever cached, so a redeployed binary is what the browser shows on the
		// next load — no stale page, no hard-refresh, no version confusion.
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// plainHost is the host a request names, without its port, when it is a
// hostname or an IPv4 address and nothing else; "" otherwise.
func plainHost(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		return host
	}
	if datatype.Validate("hostname", host) == nil {
		return host
	}
	return ""
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

// hostGuard rejects requests whose Host is not allow-listed — the app-level
// DNS-rebinding defense-in-depth (VS-01). It is opt-in: an empty allowlist
// accepts any Host (dev and tests), and $VERSO_ALLOWED_HOSTS turns it on for
// production. OpenWrt's dnsmasq rebind protection is the primary, network-layer
// defense.
//
// Cross-site request forgery is handled separately by the per-session CSRF token
// (like LuCI's sid-in-request), not by an Origin/Host comparison — which is
// fragile behind a reverse proxy, where the browser's Origin and the Host the
// shell sees legitimately differ.
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			http.Error(w, "bad host", http.StatusBadRequest)
			return
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

func bareHost(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

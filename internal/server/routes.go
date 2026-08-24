// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

// routes is the shell's routing table: every path the server exposes, in one
// scannable place. Handlers live in their feature files; this stays a map.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Shell client-side JS (htmx, Alpine, verso.js), served static and public.
	s.mux.Handle("GET /assets/", s.assets())
	s.mux.HandleFunc("GET /login", s.handleLoginForm)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.handleLogout)
	s.mux.HandleFunc("GET /{$}", s.handleIndex)

	// Shell-owned auth surface (ADR-009 §3): the shell serves the password page
	// itself, since it mutates the credential that gates the shell.
	s.mux.HandleFunc("GET /system/password", s.handlePasswordForm)
	s.mux.HandleFunc("POST /system/password", s.handlePassword)

	// Schema gateway: every plugin page and form post funnels through here and
	// is rendered via the shell's own widget renderer (ADR-006).
	s.mux.HandleFunc("GET /plugins/{id}/{path...}", s.handlePlugin)
	s.mux.HandleFunc("POST /plugins/{id}/{path...}", s.handlePlugin)
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

// routes is the shell's routing table: every path the server exposes, in one
// scannable place. Handlers live in their feature files; this stays a map.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /{$}", s.handleIndex)

	// Schema gateway: every plugin page and form post funnels through here and
	// is rendered via the shell's own widget renderer (ADR-006).
	s.mux.HandleFunc("GET /plugins/{id}/{path...}", s.handlePlugin)
	s.mux.HandleFunc("POST /plugins/{id}/{path...}", s.handlePlugin)
}

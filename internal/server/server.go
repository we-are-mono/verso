// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package server wires the Verso shell's HTTP routes and lifecycle.
//
// The Server owns routing only. Backend (ubus/uci) and plugin-transport
// dependencies are injected here as interfaces as they are introduced, so the
// shell stays unit-testable without a real OpenWrt device (see ADR-003).
package server

import "net/http"

// Server is the Verso HTTP shell.
type Server struct {
	mux *http.ServeMux
}

// New constructs a Server with its routes registered.
func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the root HTTP handler for the shell.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

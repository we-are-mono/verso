// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

// routes is the shell's routing table: every path the server exposes, in one
// scannable place. Handlers live in their feature files; this stays a map.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Shell client-side JS (htmx, Alpine, verso.js), served static and public.
	s.mux.Handle("GET /assets/", s.assets())
	// The stylesheet at a stable URL (more specific than /assets/, so it wins): the
	// page inlines CSS for first paint, and the dev hot-reload script re-fetches this.
	s.mux.HandleFunc("GET /assets/verso.css", s.handleCSS)
	s.mux.HandleFunc("GET /login", s.handleLoginForm)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.handleLogout)
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /system", s.handleSystemRoot)
	s.mux.HandleFunc("GET /system/{$}", s.handleSystemRoot)
	s.mux.HandleFunc("GET /system/general", s.handleSystemGeneral)
	s.mux.HandleFunc("POST /system/general", s.handleSystemGeneral)

	// Shell-owned auth surface (ADR-009 §3): the shell serves the password page
	// itself, since it mutates the credential that gates the shell.
	s.mux.HandleFunc("GET /system/password", s.handlePasswordForm)
	s.mux.HandleFunc("POST /system/password", s.handlePassword)
	s.mux.HandleFunc("GET /system/access", s.handlePasswordForm)
	s.mux.HandleFunc("POST /system/access", s.handlePassword)

	// Package + service management (ADR-011): shell-owned — installing or
	// stopping things mutates the set the shell trusts. Packages are files on
	// disk; Services are procd's live table — two natures, two pages. All
	// immediate acts, outside the staged-changes lifecycle.
	s.mux.HandleFunc("GET /system/packages", s.handlePackagesPage)
	s.mux.HandleFunc("POST /system/packages", s.handlePackagesAction)
	s.mux.HandleFunc("GET /system/packages/discover", s.handleDiscoverPage)
	s.mux.HandleFunc("POST /system/packages/discover", s.handleDiscoverAction)
	s.mux.HandleFunc("GET /system/services", s.handleServicesPage)
	s.mux.HandleFunc("POST /system/services", s.handleServicesAction)
	s.mux.HandleFunc("GET /system/maintenance", s.handleSystemMaintenance)

	// The overview stream (SSE): the browser's EventSource holds this open and
	// the shell pushes fresh readings into it (events.go). Read-only, behind
	// the same session gate as every page.
	s.mux.HandleFunc("GET /overview/events", s.handleOverviewEvents)

	// Staged-changes capsule (ADR-010): apply with device-side rollback, confirm
	// to disarm it, discard to revert the stage. Session- and CSRF-gated like
	// every state-changing route.
	s.mux.HandleFunc("POST /uci/apply", s.handleUCIApply)
	s.mux.HandleFunc("POST /uci/confirm", s.handleUCIConfirm)
	s.mux.HandleFunc("POST /uci/discard", s.handleUCIDiscard)

	// Schema gateway: every plugin page and form post funnels through here and
	// is rendered via the shell's own widget renderer (ADR-006).
	s.mux.HandleFunc("GET /plugins/{id}/{path...}", s.handlePlugin)
	s.mux.HandleFunc("POST /plugins/{id}/{path...}", s.handlePlugin)
}

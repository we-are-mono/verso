// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package server wires the Verso shell's HTTP routes and lifecycle.
//
// The Server owns routing only. Its side-effecting dependencies — the widget
// renderer, the OpenWrt backend, and the plugin transport — are injected as
// interfaces, so the shell stays unit-testable without a real device or a live
// plugin (see ADR-003, ADR-006).
package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed assets/verso.css
var cssText string

// scriptFS holds the shell's client-side JS, served under /assets and loaded by
// the page chrome (ADR-004). htmx drives server round-trips; Alpine (CSP build)
// drives client behaviour; verso.js registers the shell's Alpine components.
//
//go:embed assets/htmx.min.js assets/alpine.csp.min.js assets/verso.js
var scriptFS embed.FS

// Server is the Verso HTTP shell.
type Server struct {
	mux          *http.ServeMux
	widgets      *widget.Renderer
	backend      openwrt.Backend
	transport    plugin.Transport
	manifests    []plugin.Manifest
	pluginByID   map[string]plugin.Manifest
	auth         Authenticator
	security     Security
	sessions     *Sessions
	loginLimiter *loginLimiter
	allowedHosts map[string]bool
	page         *template.Template
	css          template.CSS
}

// SetAllowedHosts configures the Host allowlist for the DNS-rebinding guard
// (VS-01). An empty list leaves the check disabled; production supplies the
// device's hostnames and LAN addresses.
func (s *Server) SetAllowedHosts(hosts []string) { s.allowedHosts = hostSet(hosts) }

// New constructs a Server. It renders widgets through the injected renderer,
// reads live state through the injected backend, and reaches plugins through the
// injected transport; manifests are the plugins discovered at startup.
func New(
	widgets *widget.Renderer,
	backend openwrt.Backend,
	transport plugin.Transport,
	manifests []plugin.Manifest,
	auth Authenticator,
	security Security,
) (*Server, error) {
	page, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}
	s := &Server{
		mux:          http.NewServeMux(),
		widgets:      widgets,
		backend:      backend,
		transport:    transport,
		manifests:    manifests,
		pluginByID:   indexByID(manifests),
		auth:         auth,
		security:     security,
		sessions:     newSessions(),
		loginLimiter: newLoginLimiter(time.Now),
		page:         page,
		css:          template.CSS(cssText),
	}
	s.routes()
	return s, nil
}

func indexByID(manifests []plugin.Manifest) map[string]plugin.Manifest {
	byID := make(map[string]plugin.Manifest, len(manifests))
	for _, m := range manifests {
		byID[m.ID] = m
	}
	return byID
}

// Handler returns the root HTTP handler for the shell: security headers, then
// the session/CSRF gate, wrapping the routing mux.
func (s *Server) Handler() http.Handler {
	return securityHeaders(s.hostGuard(s.requireAuth(s.mux)))
}

// assets serves the shell's embedded client-side JS (ADR-004) under /assets. The
// files are first-party and static; they carry no session data, so the path is
// public (see isPublicPath) and cacheable.
func (s *Server) assets() http.Handler {
	sub, err := fs.Sub(scriptFS, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.FileServerFS(sub)
	return http.StripPrefix("/assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

type pageData struct {
	Title      string
	Heading    string
	CSS        template.CSS
	Nav        []navSection
	Body       template.HTML
	NoPassword bool
	CSRFToken  string
}

// renderPage wraps a rendered body in the shell chrome — the <title>, the
// manifest-driven nav with the active link marked, and the page heading — and
// sends it with the given status (200 normally; a plugin's 422 is propagated).
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, heading string, body template.HTML) {
	var buf bytes.Buffer
	if err := s.page.ExecuteTemplate(&buf, "page.html.tmpl", pageData{
		Title:      "Verso",
		Heading:    heading,
		CSS:        s.css,
		Nav:        s.buildNav(r.URL.Path),
		Body:       body,
		NoPassword: !s.security.RootHasPassword(),
		CSRFToken:  s.sessionCSRF(r),
	}); err != nil {
		http.Error(w, "page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// handleIndex renders the system-status page from live backend data. Unlike a
// plugin page, this is the shell's own content, so a render failure is a real
// 500, not a contained notice.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Wrap the status table in a headerless card, so the homepage matches the
	// plugin pages (a white, shadowed card on the gray content).
	page := &widget.Card{Children: []widget.Widget{s.statusTable(r.Context(), s.sessionSID(r))}}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, page, s.sessionCSRF(r)); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, "Overview", template.HTML(body.String()))
}

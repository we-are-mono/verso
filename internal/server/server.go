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
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed assets/verso.css
var cssText string

// Server is the Verso HTTP shell.
type Server struct {
	mux        *http.ServeMux
	widgets    *widget.Renderer
	backend    openwrt.Backend
	transport  plugin.Transport
	manifests  []plugin.Manifest
	pluginByID map[string]plugin.Manifest
	page       *template.Template
	css        template.CSS
}

// New constructs a Server. It renders widgets through the injected renderer,
// reads live state through the injected backend, and reaches plugins through the
// injected transport; manifests are the plugins discovered at startup.
func New(
	widgets *widget.Renderer,
	backend openwrt.Backend,
	transport plugin.Transport,
	manifests []plugin.Manifest,
) (*Server, error) {
	page, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}
	s := &Server{
		mux:        http.NewServeMux(),
		widgets:    widgets,
		backend:    backend,
		transport:  transport,
		manifests:  manifests,
		pluginByID: indexByID(manifests),
		page:       page,
		css:        template.CSS(cssText),
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

// Handler returns the root HTTP handler for the shell.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

type pageData struct {
	Title   string
	Heading string
	CSS     template.CSS
	Nav     []navSection
	Body    template.HTML
}

// renderPage wraps a rendered body in the shell chrome — the <title>, the
// manifest-driven nav with the active link marked, and the page heading — and
// sends it with the given status (200 normally; a plugin's 422 is propagated).
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, heading string, body template.HTML) {
	var buf bytes.Buffer
	if err := s.page.ExecuteTemplate(&buf, "page.html.tmpl", pageData{
		Title:   "Verso",
		Heading: heading,
		CSS:     s.css,
		Nav:     s.buildNav(r.URL.Path),
		Body:    body,
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
	var body strings.Builder
	if err := s.widgets.Render(&body, s.statusTable(r.Context())); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, "Overview", template.HTML(body.String()))
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package server wires the Verso shell's HTTP routes and lifecycle.
//
// The Server owns routing only. Its side-effecting dependencies — the widget
// renderer and the OpenWrt backend — are injected as interfaces, so the shell
// stays unit-testable without a real device (see ADR-003).
package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed assets/verso.css
var cssText string

// Server is the Verso HTTP shell.
type Server struct {
	mux     *http.ServeMux
	widgets *widget.Renderer
	backend openwrt.Backend
	page    *template.Template
	css     template.CSS
}

// New constructs a Server that renders widgets through the injected renderer and
// reads live state through the injected backend.
func New(widgets *widget.Renderer, backend openwrt.Backend) (*Server, error) {
	page, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}
	s := &Server{
		mux: http.NewServeMux(), widgets: widgets, backend: backend,
		page: page, css: template.CSS(cssText),
	}
	s.routes()
	return s, nil
}

// Handler returns the root HTTP handler for the shell.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

type pageData struct {
	Title string
	CSS   template.CSS
	Body  template.HTML
}

// handleIndex renders the system-status page from live backend data.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	table := s.statusTable(r.Context())

	var body strings.Builder
	if err := s.widgets.Render(&body, table); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	var page bytes.Buffer
	if err := s.page.ExecuteTemplate(&page, "page.html.tmpl", pageData{
		Title: "Verso",
		CSS:   s.css,
		Body:  template.HTML(body.String()),
	}); err != nil {
		http.Error(w, "page error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page.Bytes())
}

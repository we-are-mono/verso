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
	"os"
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
//go:embed assets/htmx.min.js assets/alpine.csp.min.js assets/verso.js assets/verso-dev.js assets/verso-boot.js
//go:embed assets/fonts
var scriptFS embed.FS

// devCSSPath is the hot-reload stylesheet drop point. When scripts/dev.sh is driving a
// session it compiles the CSS here, and the shell reads it fresh on every render (and
// serves it at /assets/verso.css for the in-place swap) — so a CSS edit lands with no
// rebuild. The file exists only under dev.sh; every real deployment has no such file and
// uses the embedded stylesheet. (Not under /tmp: the dev container mounts /tmp as tmpfs,
// which `docker cp` cannot write into.)
const devCSSPath = "/usr/share/verso/verso-dev.css"

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
	devCSS       string // dev hot-reload stylesheet path, "" in a normal build
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
	page, err := template.New("page").Funcs(template.FuncMap{"icon": widget.Icon}).ParseFS(templateFS, "templates/*.tmpl")
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
	// Enter CSS hot-reload only when the dev drop file is present (scripts/dev.sh);
	// checked once, so a normal deployment pays nothing per render.
	if _, err := os.Stat(devCSSPath); err == nil {
		s.devCSS = devCSSPath
	}
	s.routes()
	return s, nil
}

// currentCSS is the stylesheet to inline: the live dev file (read fresh each render) in
// a hot-reload session, otherwise the embedded copy. A missing or empty dev file falls
// back to embedded, so a mid-write read never blanks the page.
func (s *Server) currentCSS() template.CSS {
	if s.devCSS != "" {
		if b, err := os.ReadFile(s.devCSS); err == nil && len(b) > 0 {
			return template.CSS(b)
		}
	}
	return s.css
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
		// Fonts never change and are the one asset a re-fetch makes visible (a FOUT
		// blink on every navigation), so cache them hard. Everything else stays
		// no-cache, so a redeployed binary's JS/CSS is picked up immediately.
		if strings.HasPrefix(r.URL.Path, "fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// handleCSS serves the current stylesheet at a stable URL. The page still inlines the
// CSS for first paint; this is what the dev hot-reload script re-fetches to swap the
// <style> in place. Fresh from disk in a dev session, embedded otherwise.
func (s *Server) handleCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(s.currentCSS()))
}

type pageData struct {
	Title      string
	Heading    string
	Kicker     string // optional eyebrow above the heading (with a live dot when Live)
	Live       bool
	Subheading string // optional lede under the heading
	Width      string // content-column width preset: "narrow" | "normal" (default) | "wide"
	CSS        template.CSS
	Nav        navModel
	Body       template.HTML
	NoPassword bool
	CSRFToken  string
	Dev        bool        // dev session: inject the CSS hot-reload script
	Capsule    capsuleView // pending uci changes the staged-changes capsule shows (ADR-010)
}

// pageHeader is the masthead the shell renders above a page body. Heading is always
// shown; a page may also declare a kicker (an eyebrow, optionally with a live dot) and a
// lede subheading to get the fuller "your connection, live" header, otherwise it stays a
// plain heading.
type pageHeader struct {
	Heading    string
	Kicker     string
	Live       bool
	Subheading string
}

// renderPage wraps a rendered body in the shell chrome — the <title>, the
// manifest-driven nav with the active link marked, and the page heading — and
// sends it with the given status (200 normally; a plugin's 422 is propagated).
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, hdr pageHeader, width string, body template.HTML) {
	var buf bytes.Buffer
	if err := s.page.ExecuteTemplate(&buf, "page.html.tmpl", pageData{
		Title:      "Verso",
		Heading:    hdr.Heading,
		Kicker:     hdr.Kicker,
		Live:       hdr.Live,
		Subheading: hdr.Subheading,
		Width:      width,
		CSS:        s.currentCSS(),
		Nav:        s.buildSidebar(r.URL.Path),
		Body:       body,
		NoPassword: !s.security.RootHasPassword(),
		CSRFToken:  s.sessionCSRF(r),
		Dev:        s.devCSS != "",
		Capsule:    s.capsule(r.Context(), s.sessionSID(r)),
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
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Overview"}, "", template.HTML(body.String()))
}

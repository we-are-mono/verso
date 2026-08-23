// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// supportedSchemaVersion is the widget-schema vocabulary this shell renders. A
// plugin declaring another version degrades to a notice (ADR-006 §6), never a
// crash.
const supportedSchemaVersion = 1

// handlePlugin is the schema gateway (ADR-006 §3): it fetches a widget schema
// from the plugin's socket and renders it through the shell's own chrome and
// tokens. It is NOT a reverse proxy — plugin bytes are data the shell renders,
// never markup streamed to the browser.
func (s *Server) handlePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := s.pluginByID[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	heading := m.Name
	body, status := s.pluginBody(r, m, &heading)
	s.renderPage(w, r, status, heading, body)
}

// pluginBody returns the rendered page body for a plugin request, or a contained
// notice. Every plugin-side failure — unsupported schema version, transport
// error, undecodable schema, render error — resolves to a styled notice and a
// 200, so a third-party plugin can never 500 the shell or take it down
// (ADR-006 §7). The shell's own failures (e.g. the page template) still 500.
// pluginBody returns the rendered body and the HTTP status to send the browser.
// Notices (version mismatch, unavailable) are 200 — the shell is fine, it is
// just reporting. A plugin's own 422 (a validation failure) is propagated, so
// the HTTP semantics stay honest; everything else is 200.
func (s *Server) pluginBody(r *http.Request, m plugin.Manifest, heading *string) (template.HTML, int) {
	if m.SchemaVersion != supportedSchemaVersion {
		return s.notice("Plugin needs a newer Verso", fmt.Sprintf(
			"%s speaks schema version %d; this shell supports version %d.",
			m.Name, m.SchemaVersion, supportedSchemaVersion)), http.StatusOK
	}

	req := plugin.Request{Method: r.Method, Path: r.PathValue("path"), Query: r.URL.Query()}
	if r.Method != http.MethodGet {
		if err := r.ParseForm(); err == nil {
			req.Form = r.PostForm
		}
	}

	env, err := s.transport.Fetch(r.Context(), m.Socket, req)
	if err != nil {
		log.Printf("verso: plugin %q unavailable: %v", m.ID, err)
		return s.unavailable(m), http.StatusOK
	}
	wdg, err := widget.Decode(env.Widget)
	if err != nil {
		log.Printf("verso: plugin %q returned undecodable schema: %v", m.ID, err)
		return s.unavailable(m), http.StatusOK
	}
	var b strings.Builder
	if err := s.widgets.Render(&b, wdg); err != nil {
		log.Printf("verso: plugin %q render failed: %v", m.ID, err)
		return s.unavailable(m), http.StatusOK
	}
	if env.Title != "" {
		*heading = env.Title
	}
	status := http.StatusOK
	if env.Status == http.StatusUnprocessableEntity {
		status = http.StatusUnprocessableEntity
	}
	return template.HTML(b.String()), status
}

type noticeData struct{ Title, Message string }

// notice renders a shell-owned message (not plugin content) through the shell's
// tokens. Fields flow through html/template, so an untrusted plugin name in the
// message is escaped.
func (s *Server) notice(title, message string) template.HTML {
	var b bytes.Buffer
	if err := s.page.ExecuteTemplate(&b, "notice.html.tmpl", noticeData{title, message}); err != nil {
		return template.HTML(template.HTMLEscapeString(title + ": " + message))
	}
	return template.HTML(b.String())
}

func (s *Server) unavailable(m plugin.Manifest) template.HTML {
	return s.notice("Plugin unavailable", fmt.Sprintf(
		"%s isn’t responding right now. The rest of Verso is unaffected.", m.Name))
}

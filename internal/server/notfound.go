// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/widget"
)

// notFound answers an address with nothing at it. A page someone navigated to
// — a typo, a stale bookmark, a plugin that is gone — is answered as a page:
// the shell around it, a heading that says so, and the way home, so nobody is
// stranded on the server's bare line. A fragment, a stream or a script's fetch
// gets the bare 404 it can read.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("HX-Request") != "" || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	nothing := &widget.Empty{
		Icon:     "search",
		Title:    "There is nothing at this address",
		Body:     "The link may be old, or the address mistyped.",
		Children: []widget.Widget{&widget.Link{Label: "Go to the Overview", Href: "/", Style: "secondary"}},
	}
	var body bytes.Buffer
	if err := s.widgets.RenderWithToken(&body, nothing, s.sessionCSRF(r), lang, t); err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderPage(w, r, http.StatusNotFound, pageHeader{Heading: "Page not found"}, "", nil, template.HTML(body.String())) //nolint:gosec // rendered by the shell's own templates
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

// devStylesheetPath is the stylesheet's stable address in a dev session: read
// fresh from the drop file (devCSSPath) and never kept, so the dev script can
// swap a page's link when a block changes.
const devStylesheetPath = "/assets/verso.css"

// stylesheet is the build's compiled stylesheet as the shell serves it. Every
// page links it at an address naming its content, so a browser fetches and
// parses it once and keeps it for good, and a new build is a new address, so
// no browser shows a stale one. It is compressed once, at start, for a
// browser that takes it so.
type stylesheet struct {
	href string
	text []byte
	gzip []byte
}

func newStylesheet(text string) stylesheet {
	sum := sha256.Sum256([]byte(text))
	var packed bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression) // a valid level never errs
	_, _ = zw.Write([]byte(text))
	_ = zw.Close()
	return stylesheet{
		href: "/assets/verso." + hex.EncodeToString(sum[:])[:12] + ".css",
		text: []byte(text),
		gzip: packed.Bytes(),
	}
}

// stylesheetHref is the address a page links its stylesheet at: the build's,
// or the live one in a dev session.
func (s *Server) stylesheetHref() string {
	if s.devCSS != "" {
		return devStylesheetPath
	}
	return s.sheet.href
}

// handleStylesheet serves the build's stylesheet, kept by the browser for
// good: its address changes with its content.
func (s *Server) handleStylesheet(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/css; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("Vary", "Accept-Encoding")
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		h.Set("Content-Encoding", "gzip")
		_, _ = w.Write(s.sheet.gzip)
		return
	}
	_, _ = w.Write(s.sheet.text)
}

// handleDevStylesheet serves the live stylesheet in a dev session, fresh from
// the drop file each time (the embedded one while the file is missing or
// mid-write), with the shell's per-process id: a link swap cannot show a
// template edit (templates are embedded), so the dev script reloads the page
// when the id changes. Outside a dev session there is nothing at the address.
func (s *Server) handleDevStylesheet(w http.ResponseWriter, r *http.Request) {
	if s.devCSS == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Verso-Boot", s.bootID)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	if b, err := os.ReadFile(s.devCSS); err == nil && len(b) > 0 {
		_, _ = w.Write(b)
		return
	}
	_, _ = w.Write(s.sheet.text)
}

// acceptsGzip reports whether an Accept-Encoding header takes gzip.
func acceptsGzip(header string) bool {
	for _, coding := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(coding), ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
			return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
		}
	}
	return false
}

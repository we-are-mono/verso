// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/widget"
)

// The reader mode (ADR-015) is a preference of the person reading, not intent
// about the device, so it lives in a cookie rather than in uci (ADR-013): two
// people administering one router from two browsers hold two modes, and flipping
// the switch never passes through the staging capsule. A browser without the
// cookie reads Verso in basic mode — the product meets the least technical reader
// first.
const (
	modeCookie = "verso-mode"
	// modePath is where the switch posts. A flip is a state change like any other
	// shell form: session-gated and CSRF-checked, then a redirect back to the page
	// the reader was on.
	modePath = "/mode"
	// modeField carries the mode the switch is asking for, and modeNextField the
	// page to return to.
	modeField     = "mode"
	modeNextField = "next"
	// modeMaxAge keeps the choice for a year — long enough that a reader who turned
	// the machinery on finds it on again months later.
	modeMaxAge = 365 * 24 * time.Hour
)

// readerMode is the mode this request renders in: advanced when the browser
// carries the cookie saying so, basic for every other browser. It is read once
// per render and never consulted as authority — hiding a page protects nothing
// (ADR-015 §6).
func readerMode(r *http.Request) string {
	if cookie, err := r.Cookie(modeCookie); err == nil && cookie.Value == widget.ModeAdvanced {
		return widget.ModeAdvanced
	}
	return widget.ModeBasic
}

// modeShows reports whether content declaring one reading renders in the reader's
// mode. It is the navigation half of widget.FilterMode's rule, and states it the
// same way: only the two known tags filter, so a value the shell does not know
// renders in both modes rather than vanishing from both.
func modeShows(declared, mode string) bool {
	switch declared {
	case widget.ModeBasic:
		return mode != widget.ModeAdvanced
	case widget.ModeAdvanced:
		return mode == widget.ModeAdvanced
	}
	return true
}

// reading is the widget tree this request's reader sees: one of the shell's own
// pages, filtered by the reader's mode exactly as a plugin's page is. Every page
// the shell composes passes through it, so a mode-tagged section behaves the same
// wherever it was authored.
func (s *Server) reading(r *http.Request, root widget.Widget) widget.Widget {
	return widget.FilterMode(root, readerMode(r))
}

// handleMode flips the reader's mode and returns them to the page they were on.
// The switch is a plain form: the server renders the state, the POST changes it,
// and the next render is the truth — so basic mode never ships the advanced markup
// to be hidden in the browser.
func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	// The CSRF gate already parsed the form to check the token.
	advanced := r.PostForm.Get(modeField) == widget.ModeAdvanced
	cookie := &http.Cookie{
		Name:     modeCookie,
		Value:    widget.ModeAdvanced,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // set over HTTPS; the dev container is plain HTTP
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(modeMaxAge / time.Second),
	}
	if !advanced {
		// Basic is the absence of the preference, not a second stored value, so
		// turning the machinery off leaves the browser as it was before.
		cookie.Value, cookie.MaxAge = "", -1
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, localPath(r.PostForm.Get(modeNextField)), http.StatusSeeOther)
}

// localPath bounds where the flip returns to: a path on this shell, never an
// address a crafted form could aim the browser at. A leading "//" or "/\" is a
// protocol-relative URL to somewhere else, not a path here, so anything but a
// single leading slash lands home.
func localPath(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, `/\`) {
		return next
	}
	return "/"
}

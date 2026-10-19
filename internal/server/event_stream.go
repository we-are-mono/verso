// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/widget"
)

// eventStreamPath is the browser's one stream of the events said to its
// session (ADR-028): each the shell's own notification, which one tab holds
// and passes to the others (verso-page.js).
const eventStreamPath = "/events/stream"

// defaultEventKeepAlive is how often an idle events stream proves it is open
// and asks whether its session still is.
const defaultEventKeepAlive = 30 * time.Second

// handleEventStream sends the session's kept events newer than the one the
// browser last saw (Last-Event-ID), then each one as it is kept, until the
// browser goes or the session ends. Only an event arriving wakes it.
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	token := cookie.Value
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	pluginTr := s.pluginTranslators(r)
	send := func() bool {
		kinds := s.eventKinds()
		for _, said := range s.sessions.told(token, after) {
			k := kinds.Of(said.Event)
			what, tone := k.Words(said.Event, kindTranslator(k, tr, pluginTr))
			var fragment strings.Builder
			if err := s.pageSet(lang).ExecuteTemplate(&fragment, "verso-flash", widget.Flash{Variant: tone, Message: what}); err != nil {
				return false
			}
			if writeEvent(w, strconv.FormatInt(said.ID, 10), "said", map[string]string{"html": fragment.String()}) != nil {
				return false
			}
			after = said.ID
		}
		return true
	}
	keepAlive := s.eventKeepAlive
	if keepAlive == 0 {
		keepAlive = defaultEventKeepAlive
	}
	s.serveSSEOn(w, r, func() <-chan struct{} { return s.sessions.eventWake(token) }, keepAlive, send)
}

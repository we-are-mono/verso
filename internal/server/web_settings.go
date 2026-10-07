// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/we-are-mono/verso/internal/listen"
	"github.com/we-are-mono/verso/internal/widget"
)

// The web interface's settings on Access (ADR-017 §1): the ports the shell
// answers on and whether HTTP is sent to HTTPS, in verso.web. Each is edited in
// place and staged on its own; an apply then moves the shell's listeners while
// it serves. An option set to what its absence means is cleared, so the config
// stays a record of what differs from the router's defaults (ADR-013 §2).

const (
	webSection       = "web"
	webHTTPSField    = "web_https"
	webHTTPField     = "web_http"
	webRedirectField = "web_redirect"
	webHTTPSOption   = "listen_https"
	webHTTPOption    = "listen_http"
	webRedirectOpt   = "redirect_https"
)

// stagedWeb is verso.web as the session's stage leaves it, and whether it
// could be read.
func (s *Server) stagedWeb(r *http.Request) (map[string]any, bool) {
	snapshot, err := s.backend.UCIConfig(r.Context(), s.sessionSID(r), versoConfig)
	if err != nil {
		log.Printf("verso: web settings: %s config unavailable: %v", versoConfig, err)
		return nil, false
	}
	return asSection(snapshot[webSection]), true
}

// webPort is the port a listener list answers on, the first one's.
func webPort(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	_, port, _ := net.SplitHostPort(addrs[0])
	return port
}

// webSettingsSection is the web interface's part of Access. Beside LuCI the
// ports are where LuCI leaves them, and its own section says where; there is
// nothing here to set.
func (s *Server) webSettingsSection(r *http.Request) widget.Widget {
	if s.webPorts() == listen.PortsBeside {
		return nil
	}
	section := &widget.Section{Title: "Web interface", Hairline: true}
	web, readable := s.stagedWeb(r)
	redirect, _ := web[webRedirectOpt].(string)
	c, err := listen.New(stringList(web[webHTTPSOption]), stringList(web[webHTTPOption]), redirect)
	if !readable || err != nil {
		section.Children = []widget.Widget{&widget.Callout{Variant: "warning", Compact: true,
			Body: "Where the web interface answers could not be read just now. Reload in a moment."}}
		return section
	}
	section.Children = []widget.Widget{&widget.Settings{Items: []widget.SettingsItem{
		{
			Title: "HTTPS port", Desc: "Where this interface answers. Moving it moves this page.",
			Code: webSection + "." + webHTTPSOption, Value: webPort(c.HTTPS),
			Name: webHTTPSField, Inline: true, Datatype: "port",
		},
		{
			Title: "HTTP port", Desc: "Plain connections, sent on to HTTPS while the redirect is on.",
			Code: webSection + "." + webHTTPOption, Value: webPort(c.HTTP),
			Name: webHTTPField, Inline: true, Datatype: "port",
		},
		{
			Title:  "Redirect HTTP to HTTPS",
			Code:   webSection + "." + webRedirectOpt,
			Toggle: &widget.SettingsToggle{Name: webRedirectField, On: c.Redirect},
		},
	}}}
	return section
}

// handleWebSetting stages one of the web interface's settings, if the post
// carries one, and answers with Access read again. It reports whether the
// post was one.
func (s *Server) handleWebSetting(w http.ResponseWriter, r *http.Request) bool {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	form := r.PostForm
	var option string
	var value any
	switch {
	case form.Has(webRedirectField):
		option = webRedirectOpt
		if form.Get(webRedirectField) != "on" {
			value = "0"
		}
	case form.Has(webHTTPSField), form.Has(webHTTPField):
		field, opt, fallback, other, otherFallback := webHTTPSField, webHTTPSOption, "443", webHTTPOption, "80"
		if form.Has(webHTTPField) {
			field, opt, fallback, other, otherFallback = webHTTPField, webHTTPOption, "80", webHTTPSOption, "443"
		}
		option = opt
		port := form.Get(field)
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			http.Error(w, tr("Enter a port from 1 to 65535."), http.StatusUnprocessableEntity)
			return true
		}
		web, readable := s.stagedWeb(r)
		if !readable {
			http.Error(w, tr("That setting could not be saved just now. Try again in a moment."), http.StatusBadGateway)
			return true
		}
		otherPort := webPort(stringList(web[other]))
		if otherPort == "" {
			otherPort = otherFallback
		}
		if port == otherPort {
			http.Error(w, tr("HTTP and HTTPS need ports of their own."), http.StatusUnprocessableEntity)
			return true
		}
		value = webListeners(stringList(web[opt]), port, fallback)
	default:
		return false
	}
	ctx, sid := r.Context(), s.sessionSID(r)
	var err error
	if value == nil {
		err = s.backend.UCIDelete(ctx, sid, versoConfig, webSection, option)
	} else {
		err = s.stageVersoValue(ctx, sid, webSection, option, value)
	}
	if err != nil {
		log.Printf("verso: web settings: staging %s failed: %v", option, err)
		http.Error(w, tr("That setting could not be saved just now. Try again in a moment."), http.StatusBadGateway)
		return true
	}
	s.renderAccess(w, r, http.StatusOK, nil, "")
	return true
}

// webListeners is a listener list moved to port: each address it was bound to
// keeps its address, and an absent list is every address of both families.
// The router's own port on every address is what an absent option means, so
// it comes back as nil: the option is cleared rather than written.
func webListeners(addrs []string, port, fallback string) any {
	if len(addrs) == 0 {
		addrs = []string{"0.0.0.0:" + fallback, "[::]:" + fallback}
	}
	moved := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = ""
		}
		moved = append(moved, net.JoinHostPort(host, port))
	}
	if port == fallback && len(moved) == 2 && moved[0] == "0.0.0.0:"+port && moved[1] == "[::]:"+port {
		return nil
	}
	return moved
}

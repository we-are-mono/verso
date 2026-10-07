// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/we-are-mono/verso/internal/listen"
	"github.com/we-are-mono/verso/internal/widget"
)

// The web interface's settings on Access (ADR-017 §1): the ports the shell
// answers on and whether HTTP is sent to HTTPS, in verso.web. They apply at
// once with the section's Apply rather than staging: the new listeners are
// bound beside the open ones first, so a port that cannot be opened is said
// under its field and nothing changes; the page is then sent to where the shell
// answers. Only once the browser arrives there are the settings written and
// the old listeners closed, so a browser that never arrives — or a shell that
// restarts meanwhile — leaves the router answering where it did.

const (
	webSection       = "web"
	webHTTPSField    = "web_https"
	webHTTPField     = "web_http"
	webRedirectField = "web_redirect"
	webHTTPSOption   = "listen_https"
	webHTTPOption    = "listen_http"
	webRedirectOpt   = "redirect_https"
	// webArrivalWindow is how long the old listeners wait for the browser at
	// the new address: long enough to pass a certificate warning there.
	webArrivalWindow = 90 * time.Second
)

// webOptions is verso.web's listener options as written: nil for an option
// left out, which is what the shell's defaults stand in for.
type webOptions struct {
	HTTPS, HTTP []string
	Redirect    string
}

// webMove is a change of listeners waiting on the browser at the new address,
// and the options it writes once the browser is there.
type webMove struct {
	next    listen.Config
	options webOptions
	sid     string
}

// webMoves is the one change of listeners in flight.
type webMoves struct {
	mu      sync.Mutex
	pending *webMove
}

// stringList reads a uci option that may be a list or a single value.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

// webConfig is verso.web as written, and whether it could be read.
func (s *Server) webConfig(r *http.Request) (webOptions, bool) {
	snapshot, err := s.backend.UCIConfig(r.Context(), s.sessionSID(r), versoConfig)
	if err != nil {
		log.Printf("verso: web settings: %s config unavailable: %v", versoConfig, err)
		return webOptions{}, false
	}
	web := asSection(snapshot[webSection])
	redirect, _ := web[webRedirectOpt].(string)
	return webOptions{HTTPS: stringList(web[webHTTPSOption]), HTTP: stringList(web[webHTTPOption]), Redirect: redirect}, true
}

// webPort is the port a listener list answers on, the first one's.
func webPort(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	_, port, _ := net.SplitHostPort(addrs[0])
	return port
}

// webSettingsSection is the web interface's part of Access: its ports and the
// redirect, in one form whose Apply moves them. Beside LuCI the ports are
// where LuCI leaves them, and its own section says where; there is nothing
// here to set. A refused Apply comes back with what was typed.
func (s *Server) webSettingsSection(r *http.Request, errs map[string]string) widget.Widget {
	if s.webPorts() == listen.PortsBeside {
		return nil
	}
	section := &widget.Section{Title: "Web interface", Hairline: true}
	web, readable := s.webConfig(r)
	c, err := listen.New(web.HTTPS, web.HTTP, web.Redirect)
	if !readable || err != nil {
		section.Children = []widget.Widget{&widget.Callout{Variant: "warning", Compact: true,
			Body: "Where the web interface answers could not be read just now. Reload in a moment."}}
		return section
	}
	https, plain, redirect := webPort(c.HTTPS), webPort(c.HTTP), c.Redirect
	if r.Method == http.MethodPost && r.PostForm.Has(webHTTPSField) {
		https, plain, redirect = r.PostForm.Get(webHTTPSField), r.PostForm.Get(webHTTPField), r.PostForm.Get(webRedirectField) == "1"
	}
	section.Children = []widget.Widget{&widget.Form{
		Style: "settings", Action: "/system/access", Submit: "Apply", Target: versoConfig + "." + webSection,
		Error: errs["web"],
		Fields: []widget.Widget{
			&widget.Field{Name: webHTTPSField, Label: "HTTPS port", Key: webHTTPSOption, Datatype: "port", Value: https, Error: errs[webHTTPSField]},
			&widget.Field{Name: webHTTPField, Label: "HTTP port", Key: webHTTPOption, Datatype: "port", Value: plain, Error: errs[webHTTPField]},
			&widget.Switch{Name: webRedirectField, Label: "Redirect HTTP to HTTPS", Key: webRedirectOpt, On: redirect},
		},
	}}
	return section
}

// webListeners is a listener list moved to port: each address it was bound to
// keeps its address, and an absent list is every address of both families.
// The router's own port on every address is what an absent option means, so
// it comes back as nil: the option is cleared rather than written.
func webListeners(addrs []string, port, fallback string) []string {
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

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535 && strconv.Itoa(n) == port
}

// servedHere reports whether c answers the page where it was reached: a
// listener of its scheme on the port it came in on, at its address or every
// address. On HTTP with the redirect on, the page's next request would be
// sent away, so it is not.
func servedHere(c listen.Config, r *http.Request) bool {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || local == nil {
		return true
	}
	host, port, err := net.SplitHostPort(local.String())
	if err != nil {
		return true
	}
	addrs := c.HTTPS
	if r.TLS == nil {
		if c.Redirect {
			return false
		}
		addrs = c.HTTP
	}
	for _, addr := range addrs {
		h, p, err := net.SplitHostPort(addr)
		if err == nil && p == port && (h == "" || h == "0.0.0.0" || h == "::" || h == host) {
			return true
		}
	}
	return false
}

// moveTo is where the page goes once c answers: the first HTTPS listener it
// names, at the host this browser reached the router by.
func moveTo(c listen.Config, r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	port := webPort(c.HTTPS)
	if port == "443" {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return "https://" + host
	}
	return "https://" + net.JoinHostPort(host, port)
}

// refusedPort says why a listener could not be opened, under the field of
// the port it was for.
func refusedPort(err error, https, plain string, tr func(string) string) (string, string) {
	field, port := webHTTPField, plain
	if strings.HasPrefix(err.Error(), "https ") {
		field, port = webHTTPSField, https
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return field, fmt.Sprintf(tr("Port %s is in use on this router."), port)
	}
	return field, fmt.Sprintf(tr("Port %s can’t be opened on this router."), port)
}

// handleWebListeners applies the web interface's settings: checked, bound
// beside the open listeners, written to verso.web, and the page sent to where
// the shell now answers. The form posts to Access itself, so a refusal is
// drawn at Access's own address.
func (s *Server) handleWebListeners(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if s.listeners == nil || s.webPorts() == listen.PortsBeside {
		http.Error(w, tr("The web interface can’t be changed here now. Reload the page."), http.StatusConflict)
		return
	}
	https, plain := r.PostForm.Get(webHTTPSField), r.PostForm.Get(webHTTPField)
	redirect := ""
	if r.PostForm.Get(webRedirectField) != "1" {
		redirect = "0"
	}
	errs := map[string]string{}
	for field, port := range map[string]string{webHTTPSField: https, webHTTPField: plain} {
		if !validPort(port) {
			errs[field] = tr("Enter a port from 1 to 65535.")
		}
	}
	if len(errs) == 0 && https == plain {
		errs[webHTTPField] = tr("HTTP and HTTPS need ports of their own.")
	}
	prev, readable := s.webConfig(r)
	if !readable {
		errs["web"] = tr("Where the web interface answers could not be read just now. Reload in a moment.")
	}
	if len(errs) > 0 {
		s.renderAccess(w, r, http.StatusUnprocessableEntity, errs, "")
		return
	}
	next := webOptions{
		HTTPS:    webListeners(prev.HTTPS, https, "443"),
		HTTP:     webListeners(prev.HTTP, plain, "80"),
		Redirect: redirect,
	}
	c, err := listen.New(next.HTTPS, next.HTTP, next.Redirect)
	if err != nil {
		s.renderAccess(w, r, http.StatusUnprocessableEntity, map[string]string{"web": err.Error()}, "")
		return
	}
	now := s.listeners.Current()
	if slices.Equal(c.HTTPS, now.HTTPS) && slices.Equal(c.HTTP, now.HTTP) && c.Redirect == now.Redirect {
		http.Redirect(w, r, "/system/access", http.StatusSeeOther)
		return
	}
	// The new listeners answer before anything is written, so a port that
	// cannot be opened changes nothing.
	if err := s.listeners.Stage(c); err != nil {
		field, message := refusedPort(err, https, plain, tr)
		s.renderAccess(w, r, http.StatusUnprocessableEntity, map[string]string{field: message}, "")
		return
	}
	move := webMove{next: c, options: next, sid: s.sessionSID(r)}
	if servedHere(c, r) {
		if err := s.keepWebMove(r.Context(), move); err != nil {
			s.renderAccess(w, r, http.StatusBadGateway, map[string]string{"web": tr("The router couldn’t save where the web interface answers. Try again in a moment.")}, "")
			return
		}
		s.flash(r, "success", tr("Web interface settings applied."))
		http.Redirect(w, r, "/system/access", http.StatusSeeOther)
		return
	}
	s.awaitArrival(move)
	s.flash(r, "success", fmt.Sprintf(tr("The web interface moved to port %s."), webPort(c.HTTPS)))
	http.Redirect(w, r, moveTo(c, r)+"/system/access", http.StatusSeeOther)
}

// keepWebMove writes a move's settings and closes the listeners it leaves
// behind. A write the router refuses drops the new listeners instead, and the
// shell answers where verso.web says it does.
func (s *Server) keepWebMove(ctx context.Context, m webMove) error {
	if err := s.backend.SetWebListeners(ctx, m.sid, m.options.HTTPS, m.options.HTTP, m.options.Redirect); err != nil {
		log.Printf("verso: web settings: %v", err)
		s.listeners.Drop()
		return err
	}
	s.listeners.Keep()
	return nil
}

// awaitArrival holds the old listeners open until the browser reaches the
// new ones. Should it not within the window, the new listeners close; nothing
// was written, so nothing is written back.
func (s *Server) awaitArrival(m webMove) {
	s.webMoves.mu.Lock()
	s.webMoves.pending = &m
	s.webMoves.mu.Unlock()
	s.afterRollback(webArrivalWindow, func() {
		s.webMoves.mu.Lock()
		pending := s.webMoves.pending == &m
		if pending {
			s.webMoves.pending = nil
		}
		s.webMoves.mu.Unlock()
		if pending {
			s.listeners.Drop()
		}
	})
}

// webArrived writes the move's settings and closes the old listeners once the
// browser has reached the new ones: the page it was sent to, served on one of
// them.
func (s *Server) webArrived(r *http.Request) {
	s.webMoves.mu.Lock()
	m := s.webMoves.pending
	if m != nil && servedHere(m.next, r) {
		s.webMoves.pending = nil
	} else {
		m = nil
	}
	s.webMoves.mu.Unlock()
	if m != nil {
		_ = s.keepWebMove(r.Context(), *m)
	}
}

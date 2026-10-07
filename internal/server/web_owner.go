// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/listen"
	"github.com/we-are-mono/verso/internal/widget"
)

// Verso beside LuCI (ADR-017 §2): on a router where LuCI's uhttpd holds the web
// ports, Verso answers beside it on 8443 and Access offers to take them; once
// Verso holds them, Access offers to hand them back. Both acts move the address
// this page is on, so each asks first and ends on a takeover that follows the
// shell to where it will answer.

const webOwnerPath = "/system/access/web-owner"

// luciInstalled reports whether the router has uhttpd, the web server LuCI
// runs on, to hand the web ports to or take them from.
func (s *Server) luciInstalled(r *http.Request) bool {
	rc, err := s.backend.RCList(r.Context(), s.sessionSID(r))
	if err != nil {
		return false
	}
	_, ok := rc["uhttpd"]
	return ok
}

// webOwnerMove is the owner the router's web ports can move to from where
// they are: Verso's when it answers beside LuCI, LuCI's when Verso holds its
// own. Listeners set in verso.web, or no LuCI, move nowhere.
func (s *Server) webOwnerMove(r *http.Request) string {
	if !s.luciInstalled(r) {
		return ""
	}
	switch s.webPorts() {
	case listen.PortsBeside:
		return "verso"
	case listen.PortsOwn:
		return "luci"
	}
	return ""
}

// luciSection is LuCI's part of Access: where it stands beside Verso, and the
// act that moves the router's web address between them, asked in marigold
// because it is disruptive but wanted.
func (s *Server) luciSection(r *http.Request) widget.Widget {
	var text string
	var confirm *widget.Confirm
	move := s.webOwnerMove(r)
	switch move {
	case "verso":
		text = "LuCI’s web server holds the router’s web address, so this interface answers beside it on port 8443."
		confirm = &widget.Confirm{
			Trigger: "Make this the router’s web interface",
			Title:   "Make this the router’s web interface?",
			Message: "This interface takes the router’s web address and LuCI moves to port 8443. This page moves with it; sign in again there.",
			Confirm: "Switch over", Cancel: "Not now", Tone: widget.ToneCaution,
		}
	case "luci":
		text = "LuCI is installed and answers on port 8443."
		confirm = &widget.Confirm{
			Trigger: "Hand the web interface back to LuCI",
			Title:   "Hand the web interface back to LuCI?",
			Message: "LuCI takes the router’s web address and this interface moves to port 8443. This page moves with it; sign in again there.",
			Confirm: "Hand back to LuCI", Cancel: "Not now", Tone: widget.ToneCaution,
		}
	default:
		return nil
	}
	return &widget.Section{Title: "LuCI", Hairline: true, Children: []widget.Widget{
		&widget.Text{Markdown: text},
		&widget.Form{Action: webOwnerPath, NoSubmit: true, Fields: []widget.Widget{
			&widget.Field{Name: "owner", Kind: "hidden", Value: move},
			confirm,
		}},
	}}
}

// webOwnerTarget is where Verso answers once the ports have moved: the
// router's own HTTPS port when Verso takes them, 8443 beside LuCI when it
// hands them back. The host is the one this browser reached the router by.
func webOwnerTarget(r *http.Request, owner string) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if owner == "luci" {
		return "https://" + net.JoinHostPort(host, listen.BesidePort) + "/"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "https://" + host + "/"
}

// handleWebOwner moves the router's web ports. The takeover is rendered while
// the shell still answers; the router then stops this shell, so the takeover
// watches it go and follows it to the address it answers at.
func (s *Server) handleWebOwner(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	owner := r.PostFormValue("owner")
	if owner == "" || owner != s.webOwnerMove(r) {
		http.Error(w, tr("The web interface can’t move that way now. Reload the page."), http.StatusConflict)
		return
	}
	plan := restartPlan{
		Title:    tr("Moving the web interface"),
		Lede:     tr("LuCI moves to port 8443 and this interface takes the router’s web address."),
		Estimate: tr("Usually a few seconds."),
		Budget:   time.Minute,
		Target:   webOwnerTarget(r, owner),
	}
	if owner == "luci" {
		plan.Lede = tr("LuCI takes the router’s web address and this interface moves to port 8443.")
	}
	page, err := s.restartingPage(r, plan)
	if err != nil {
		http.Error(w, "moving page error", http.StatusInternalServerError)
		return
	}
	if err := s.backend.SetWebOwner(r.Context(), s.sessionSID(r), owner); err != nil {
		log.Printf("verso: web owner: %v", err)
		http.Error(w, tr("The router couldn’t move the web interface."), http.StatusBadGateway)
		return
	}
	writeRestarting(w, page)
}

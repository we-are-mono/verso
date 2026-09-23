// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/widget"
)

const minPasswordLen = 8

// credentialVerifier is narrower than Authenticator. Login retains an rpcd sid;
// password confirmation only needs a yes/no answer and destroys its probe sid.
type credentialVerifier interface {
	Verify(context.Context, string, string) error
}

// Access is shell-owned (ADR-009 §3). It changes the credential that gates
// Verso and manages Verso's sessions. Both actions are immediate and never
// create UCI stage entries; an unrelated existing global stage may still appear.

func accessForm(hasPassword bool, username string, fieldErrs map[string]string, formErr, success string) *widget.Form {
	fields := make([]widget.Widget, 0, 4)
	// The account the password belongs to, for the browser's password manager.
	fields = append(fields, &widget.Field{Name: "username", Kind: "hidden", Value: username, Autocomplete: "username"})
	if hasPassword {
		fields = append(fields, &widget.Field{Name: "current_password", Label: "Current password", Kind: "password", Autocomplete: "current-password", Error: fieldErrs["current_password"]})
	}
	fields = append(fields, &widget.Field{Name: "password", Label: "New password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["password"]}, &widget.Field{Name: "confirm", Label: "Repeat new password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["confirm"]})
	label := "Set password"
	if hasPassword {
		label = "Change password"
	}
	return &widget.Form{Style: "settings", Action: "/system/access", Submit: label, Success: success, Error: formErr, Fields: fields, Note: "Takes effect immediately — other sessions stay signed in."}
}
func accessBody(hasPassword bool, username string, fieldErrs map[string]string, formErr, success string, sessions []accessSession, tr func(string) string) *widget.Stack {
	return &widget.Stack{Children: []widget.Widget{
		&widget.Section{Title: "Router password", Children: []widget.Widget{accessForm(hasPassword, username, fieldErrs, formErr, success)}},
		&widget.Section{Title: "Signed in now", Hairline: true, Children: []widget.Widget{accessSessionsTable(sessions, tr)}},
	}}
}

type accessSession struct {
	ID, Browser, Address, SignedIn, LastActive string
	Current                                    bool
}

func accessSessionsTable(sessions []accessSession, tr func(string) string) *widget.Table {
	rows := make([]widget.TableRow, 0, len(sessions))
	for _, session := range sessions {
		source := widget.TableCell{Text: session.Address, Sub: session.Browser}
		action := widget.TableCell{}
		if session.Current {
			source.Tag = tr("this browser")
			source.TagVariant = "success"
		} else {
			action = widget.TableCell{Button: tr("Revoke"), Name: "_action", Action: "end-session:" + session.ID, Confirm: tr("Revoke this session?"), ConfirmTitle: tr("Revoke session"), Variant: "danger"}
		}
		rows = append(rows, widget.TableRow{ID: session.ID, Cells: []widget.TableCell{source, {Text: session.SignedIn}, {Text: session.LastActive}, action}})
	}
	return &widget.Table{Columns: []widget.TableColumn{{Label: "Source", Kind: "reference"}, {Label: "Started", Kind: "mono"}, {Label: "Last seen", Kind: "mono"}, {Kind: "pill"}}, Rows: rows}
}

func (s *Server) accessSessions(r *http.Request) []accessSession {
	current, _ := s.currentSession(r)
	now := time.Now()
	items := s.sessions.list()
	sort.Slice(items, func(i, j int) bool {
		if items[i].id == current.id {
			return true
		}
		if items[j].id == current.id {
			return false
		}
		return items[i].lastSeen.After(items[j].lastSeen)
	})
	// The sessions table sets these in machine-kind columns the schema walk
	// leaves verbatim, and they compose prose with data — so they localize
	// here, at the point of composition.
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	out := make([]accessSession, 0, len(items))
	for _, sess := range items {
		address := sess.address
		if address == "" {
			address = "—"
		}
		out = append(out, accessSession{
			ID: sess.id, Browser: browserLabel(tr, sess.agent), Address: address,
			SignedIn: formatSessionStart(tr, sess.created, now), LastActive: relativeSessionTime(tr, sess.lastSeen, now),
			Current: sess.id == current.id,
		})
	}
	return out
}

func browserLabel(tr func(string) string, ua string) string {
	browser := tr("Unknown browser")
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	platform := ""
	switch {
	case strings.Contains(ua, "iPhone"):
		platform = "iPhone"
	case strings.Contains(ua, "iPad"):
		platform = "iPad"
	case strings.Contains(ua, "Android"):
		platform = "Android"
	case strings.Contains(ua, "Windows"):
		platform = "Windows"
	case strings.Contains(ua, "Macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "Linux"):
		platform = "Linux"
	}
	if platform != "" {
		return browser + " · " + platform
	}
	return browser
}

func formatSessionStart(tr func(string) string, created, now time.Time) string {
	cy, cm, cd := created.Date()
	ny, nm, nd := now.Date()
	if cy == ny && cm == nm && cd == nd {
		return fmt.Sprintf(tr("Today, %s"), created.Format("15:04"))
	}
	return created.Format("2 Jan, 15:04")
}

// relativeSessionTime words a session's age. Two flat forms per unit (one, and
// many) — the same plural shape the staged-changes chip carries, with the same
// TODO(i18n plurals) caveat: languages with more plural forms than two render
// the "many" form for all of them.
func relativeSessionTime(tr func(string) string, last, now time.Time) string {
	age := now.Sub(last)
	if age < time.Minute {
		return tr("Now")
	}
	if age < time.Hour {
		return fmt.Sprintf(tr("%d min ago"), int(age/time.Minute))
	}
	if age < 24*time.Hour {
		if h := int(age / time.Hour); h != 1 {
			return fmt.Sprintf(tr("%d hours ago"), h)
		}
		return tr("1 hour ago")
	}
	if d := int(age / (24 * time.Hour)); d != 1 {
		return fmt.Sprintf(tr("%d days ago"), d)
	}
	return tr("1 day ago")
}

func (s *Server) handlePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.renderAccess(w, r, http.StatusOK, nil, "", "")
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {

	if err := r.ParseForm(); err != nil {
		s.renderAccess(w, r, http.StatusBadRequest, nil, "Could not read the form.", "")
		return
	}
	if action := r.PostForm.Get("_action"); strings.HasPrefix(action, "end-session:") {
		s.handleEndSession(w, r, strings.TrimPrefix(action, "end-session:"))
		return
	}

	if id := r.URL.Query().Get("plugin"); id != "" {
		if m, ok := s.manifestByID(id); !ok || m.SystemAccess == "" {
			s.renderAccess(w, r, http.StatusBadRequest, nil, "These settings are no longer available. Reload the page.", "")
			return
		}
		s.renderAccess(w, r, http.StatusOK, nil, "", "")
		return
	}

	hasPassword, err := s.backend.RootHasPassword(r.Context(), s.sessionSID(r))
	if err != nil {
		s.renderAccess(w, r, http.StatusInternalServerError, nil, "Couldn’t check the administrator account just now.", "")
		return
	}
	fieldErrs := validatePassword(r.PostForm.Get("password"), r.PostForm.Get("confirm"))
	if hasPassword {
		current := r.PostForm.Get("current_password")
		switch verifier := s.auth.(type) {
		case credentialVerifier:
			if current == "" || verifier.Verify(r.Context(), s.sessionUser(r), current) != nil {
				fieldErrs["current_password"] = "Current password is incorrect."
			}
		default:
			fieldErrs["current_password"] = "Current password could not be verified."
		}
	}
	if len(fieldErrs) > 0 {
		s.renderAccess(w, r, http.StatusUnprocessableEntity, fieldErrs, "", "")
		return
	}

	password := r.PostForm.Get("password")
	if err := s.backend.SetPassword(r.Context(), s.sessionSID(r), s.sessionUser(r), password); err != nil {
		log.Printf("verso: set password failed: %v", err)
		s.renderAccess(w, r, http.StatusInternalServerError, nil, "The password couldn’t be changed just now. Try again in a moment.", "")
		return
	}
	s.renderAccess(w, r, http.StatusOK, nil, "", "Password updated.")
}

func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request, id string) {
	current, _ := s.currentSession(r)
	if id == "" || id == current.id || !s.sessions.destroyID(id) {
		s.renderAccess(w, r, http.StatusBadRequest, nil, "That session could not be ended.", "")
		return
	}
	s.flash(r, "success", "Session ended.")
	http.Redirect(w, r, "/system/access", http.StatusSeeOther)
}

func validatePassword(password, confirm string) map[string]string {
	errs := map[string]string{}
	switch {
	case password == "":
		errs["password"] = "Enter a new password."
	case len(password) < minPasswordLen:
		errs["password"] = "Use at least 8 characters."
	case confirm != password:
		errs["confirm"] = "Passwords do not match."
	}
	return errs
}

func (s *Server) renderAccess(w http.ResponseWriter, r *http.Request, status int, fieldErrs map[string]string, formErr, success string) {
	hasPassword, err := s.backend.RootHasPassword(r.Context(), s.sessionSID(r))
	if err != nil {
		hasPassword = true
		if formErr == "" {
			formErr = "Couldn’t check the administrator account just now."
		}
	}
	username := s.sessionUser(r)
	if username == "" {
		username = "root"
	}
	var body strings.Builder
	lang, t := s.localize(r)
	access := accessBody(hasPassword, username, fieldErrs, formErr, success, s.accessSessions(r), translatorOrIdentity(t))
	if err := s.widgets.RenderWithToken(&body, s.reading(r, access), s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	hdr := pageHeader{Heading: "Access", Tone: "neutral", Ruled: true}
	for _, manifest := range s.manifestList() {
		if manifest.SystemAccess == "" {
			continue
		}
		request := r.Clone(r.Context())
		if r.URL.Query().Get("plugin") != manifest.ID {
			request.Method = http.MethodGet
			request.Body = nil
			request.Form = nil
			request.PostForm = nil
		}
		var pluginHeader pageHeader
		var pluginWidth string
		var pluginPages []pageTab
		contribution, code := s.pluginBodyAt(request, manifest, manifest.SystemAccess, &pluginHeader, &pluginWidth, &pluginPages)
		if err := s.pageSet(lang).ExecuteTemplate(&body, "access-contribution.html.tmpl", contribution); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
			return
		}
		if code >= http.StatusBadRequest {
			status = code
		}
		if pluginHeader.Notice != nil {
			hdr.Notice = pluginHeader.Notice
		}
	}

	s.renderPage(w, r, status, hdr, "form", s.systemPages(r.URL.Path, readerMode(r)), template.HTML(body.String()))
}

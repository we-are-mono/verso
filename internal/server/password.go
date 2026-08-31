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
	"strconv"
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

func accessForm(hasPassword bool, fieldErrs map[string]string, formErr, success string) *widget.Form {
	fields := make([]widget.Widget, 0, 2)
	if hasPassword {
		fields = append(fields, &widget.Grid{Style: "form", Columns: 3, Children: []widget.Widget{
			&widget.Field{Name: "current_password", Label: "Current password", Kind: "password", Autocomplete: "current-password", Error: fieldErrs["current_password"]},
		}})
	}
	fields = append(fields, &widget.Grid{Style: "form", Columns: 3, Children: []widget.Widget{
		&widget.Field{Name: "password", Label: "New password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["password"]},
		&widget.Field{Name: "confirm", Label: "Repeat new password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["confirm"]},
	}})
	label := "Set password"
	if hasPassword {
		label = "Update password"
	}
	return &widget.Form{Submit: label, Success: success, Error: formErr, Fields: fields}
}

func accessBody(hasPassword bool, username string, fieldErrs map[string]string, formErr, success string, sessions []accessSession, tr func(string) string) *widget.Stack {
	return &widget.Stack{Children: []widget.Widget{
		&widget.Section{
			Title: "Administrator account",
			Sub:   "The root password is used to sign in to Verso and approve sensitive actions.",
			Children: []widget.Widget{
				&widget.Stack{Compact: true, Children: []widget.Widget{
					&widget.Properties{Style: "identity", Items: []widget.Property{{Label: "Username", Value: username, Mono: true, Emphasis: true}}},
					&widget.Callout{Variant: "neutral", Compact: true, Body: "Main system username cannot be changed."},
				}},
				accessForm(hasPassword, fieldErrs, formErr, success),
			},
		},
		&widget.Section{
			Title:    "Active sessions",
			Sub:      "Browsers currently signed in to Verso. End anything you do not recognize.",
			Hairline: true,
			Children: []widget.Widget{accessSessionsTable(sessions, tr)},
		},
	}}
}

type accessSession struct {
	ID, Browser, Address, SignedIn, LastActive string
	Current                                    bool
}

func accessSessionsTable(sessions []accessSession, tr func(string) string) *widget.Table {
	rows := make([]widget.TableRow, 0, len(sessions))
	for _, sess := range sessions {
		var access widget.TableCell
		if sess.Current {
			access = widget.TableCell{Text: "this session", Variant: "success"}
		} else {
			access = widget.TableCell{
				Button: "End session", Action: "end-session:" + sess.ID,
				ConfirmTitle: "End this session?",
				Confirm:      fmt.Sprintf(tr("Anyone using %s will be signed out of Verso immediately. They’ll need the administrator password to sign in again."), sess.Browser),
			}
		}
		rows = append(rows, widget.TableRow{ID: sess.ID, Cells: []widget.TableCell{
			{Text: sess.Browser}, {Text: sess.Address, Emphasis: true},
			{Text: sess.SignedIn}, {Text: sess.LastActive}, access,
		}})
	}
	return &widget.Table{Style: "flat", Columns: []widget.TableColumn{
		{Label: "Browser", Kind: "name"}, {Label: "Address", Kind: "mono"},
		{Label: "Signed in", Kind: "num"}, {Label: "Last active", Kind: "num"},
		{Label: "Access", Kind: "pill"},
	}, Rows: rows}
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
	out := make([]accessSession, 0, len(items))
	for _, sess := range items {
		address := sess.address
		if address == "" {
			address = "—"
		}
		out = append(out, accessSession{
			ID: sess.id, Browser: browserLabel(sess.agent), Address: address,
			SignedIn: formatSessionStart(sess.created, now), LastActive: relativeSessionTime(sess.lastSeen, now),
			Current: sess.id == current.id,
		})
	}
	return out
}

func browserLabel(ua string) string {
	browser := "Unknown browser"
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

func formatSessionStart(created, now time.Time) string {
	cy, cm, cd := created.Date()
	ny, nm, nd := now.Date()
	if cy == ny && cm == nm && cd == nd {
		return "Today, " + created.Format("15:04")
	}
	return created.Format("2 Jan, 15:04")
}

func relativeSessionTime(last, now time.Time) string {
	age := now.Sub(last)
	if age < time.Minute {
		return "Now"
	}
	if age < time.Hour {
		return strconv.Itoa(int(age/time.Minute)) + " min ago"
	}
	if age < 24*time.Hour {
		return durationWords(int(age/time.Hour), "hour") + " ago"
	}
	return durationWords(int(age/(24*time.Hour)), "day") + " ago"
}

func durationWords(n int, unit string) string {
	if n != 1 {
		unit += "s"
	}
	return strconv.Itoa(n) + " " + unit
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
	if err := s.widgets.RenderWithToken(&body, access, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	hdr := pageHeader{Heading: "System", Immediate: true, Subheading: "Control who can sign in to this router, and end access you no longer recognize."}
	s.renderPage(w, r, status, hdr, "narrow", s.systemPages(r.URL.Path), false, template.HTML(body.String()))
}

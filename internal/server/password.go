// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/widget"
)

// minPasswordLen is the shortest password the shell accepts. OpenWrt itself
// enforces none (a fresh device is passwordless); the shell adds a modest floor
// so the operator cannot lock a router behind a trivial secret by accident.
const minPasswordLen = 8

// The password page is a shell-owned auth surface (ADR-009 §3): the shell serves
// it directly, not through a plugin, because it mutates the credential that gates
// the shell. It matches LuCI's Router Password page — new password plus a
// confirmation, no current-password prompt (a fresh device has none to give).

// passwordCard builds the Router Password page. fieldErrs carries per-field
// inline errors; formErr and success are the form-level banners.
func passwordCard(fieldErrs map[string]string, formErr, success string) *widget.Card {
	return &widget.Card{
		Title: "Router Password",
		Children: []widget.Widget{&widget.Form{
			Submit:  "Save password",
			Success: success,
			Error:   formErr,
			Fields: []widget.Widget{
				&widget.Field{
					Name: "password", Label: "New password", Kind: "password",
					Help: "At least 8 characters.", Error: fieldErrs["password"],
				},
				&widget.Field{
					Name: "confirm", Label: "Confirm new password", Kind: "password",
					Error: fieldErrs["confirm"],
				},
			},
		}},
	}
}

// handlePasswordForm serves the empty Router Password form.
func (s *Server) handlePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.renderPassword(w, r, http.StatusOK, passwordCard(nil, "", ""))
}

// handlePassword validates the submitted passwords and, on success, sets the
// operator's system password. The shell owns the page (ADR-009 §3), but the
// privileged write goes through rpcd carrying the session sid (ADR-007) — the
// shell holds no ambient privilege to write /etc/shadow itself. Validation
// failures re-render the form at 422 with inline errors; a backend failure
// re-renders with a form-level notice at 500. The password is never echoed back.
func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderPassword(w, r, http.StatusBadRequest, passwordCard(nil, "Could not read the form.", ""))
		return
	}
	password := r.PostForm.Get("password")
	confirm := r.PostForm.Get("confirm")

	if fieldErrs := validatePassword(password, confirm); len(fieldErrs) > 0 {
		s.renderPassword(w, r, http.StatusUnprocessableEntity, passwordCard(fieldErrs, "", ""))
		return
	}

	if err := s.backend.SetPassword(r.Context(), s.sessionSID(r), s.sessionUser(r), password); err != nil {
		log.Printf("verso: set password failed: %v", err)
		s.renderPassword(w, r, http.StatusInternalServerError,
			passwordCard(nil, "The password couldn’t be changed just now. Try again in a moment.", ""))
		return
	}
	s.renderPassword(w, r, http.StatusOK, passwordCard(nil, "", "Password updated."))
}

// validatePassword returns per-field errors, empty when the input is acceptable.
// Length is checked before the match so the operator fixes one problem at a time.
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

func (s *Server) renderPassword(w http.ResponseWriter, r *http.Request, status int, card *widget.Card) {
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, card, s.sessionCSRF(r)); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, status, "Router Password", "narrow", template.HTML(body.String()))
}

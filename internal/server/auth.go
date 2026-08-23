// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"html/template"
	"log"
	"net/http"
	"time"
)

// Authenticator verifies credentials and returns an rpcd session id. Injected
// (ADR-003): a fake in tests, the native rpcd implementation in production.
type Authenticator interface {
	Login(ctx context.Context, username, password string) (sid string, err error)
}

// Security answers host security questions that drive in-app warnings.
type Security interface {
	RootHasPassword() bool
}

func isPublicPath(p string) bool {
	return p == "/login" || p == "/healthz"
}

// requireAuth redirects unauthenticated requests to the login page; public paths
// (login, health) pass through. It wraps the whole mux, so every page and every
// plugin write is gated behind a session.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := s.currentSession(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// CSRF: every state-changing request must carry the session's token
		// (VS-04). GET/HEAD are safe; /login is public and covered by the Origin
		// check instead (it has no session yet).
		if !safeMethod(r.Method) && !validCSRF(r, sess.csrf) {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	return s.sessions.get(cookie.Value)
}

func (s *Server) sessionCSRF(r *http.Request) string {
	if sess, ok := s.currentSession(r); ok {
		return sess.csrf
	}
	return ""
}

// sessionSID returns the rpcd session id for the request's session — the
// credential Verso presents to rpcd for ACL-gated backend calls (ADR-007).
func (s *Server) sessionSID(r *http.Request) string {
	if sess, ok := s.currentSession(r); ok {
		return sess.sid
	}
	return ""
}

func validCSRF(r *http.Request, want string) bool {
	if want == "" {
		return false
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.PostForm.Get("_csrf")), []byte(want)) == 1
}

type loginData struct {
	CSS   template.CSS
	Error string
}

// handleLoginForm serves the login page (redirecting an already-signed-in user
// home).
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentSession(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, http.StatusOK, "")
}

// handleLogin authenticates and, on success, stores the session server-side and
// hands the browser the opaque cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	key := clientIP(r)
	if !s.loginLimiter.allowed(key) {
		s.renderLogin(w, http.StatusTooManyRequests, "Too many attempts — wait a minute and try again.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, http.StatusOK, "Could not read the form.")
		return
	}
	username := r.PostForm.Get("username")

	sid, err := s.auth.Login(r.Context(), username, r.PostForm.Get("password"))
	if err != nil {
		// One generic message regardless of cause — do not reveal which of the
		// username or password was wrong. The detail is logged, not shown.
		s.loginLimiter.fail(key)
		log.Printf("verso: login failed for %q: %v", username, err)
		s.renderLogin(w, http.StatusOK, "Invalid username or password.")
		return
	}
	s.loginLimiter.success(key)
	token, err := s.sessions.Create(sid, username)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // set over HTTPS; the dev container is plain HTTP
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionAbsoluteTimeout / time.Second),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout destroys the session and clears the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.destroy(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, status int, errMsg string) {
	var buf bytes.Buffer
	if err := s.page.ExecuteTemplate(&buf, "login.html.tmpl", loginData{CSS: s.css, Error: errMsg}); err != nil {
		http.Error(w, "login page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

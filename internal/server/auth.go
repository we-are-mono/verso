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
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/version"
)

// Authenticator verifies credentials and returns an rpcd session id. Injected
// (ADR-003): a fake in tests, the native rpcd implementation in production.
type Authenticator interface {
	Login(ctx context.Context, username, password string) (sid string, err error)
}

func isPublicPath(p string) bool {
	return p == "/login" || p == "/healthz" || strings.HasPrefix(p, "/assets/")
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
		// (VS-04). GET/HEAD are safe. /login is public (isPublicPath handles it
		// above), so it never reaches this gate — it holds no session and no token
		// yet; there is no separate Origin check on it.
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

// sessionUser returns the logged-in username for the request's session — the
// account whose credential a shell-owned auth page acts on (ADR-009 §3).
func (s *Server) sessionUser(r *http.Request) string {
	if sess, ok := s.currentSession(r); ok {
		return sess.username
	}
	return ""
}

// flash stores a one-shot confirmation on the request's session — set by an
// action just before its redirect, shown by the next render (the PRG flash).
func (s *Server) flash(r *http.Request, variant, message string) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.SetFlash(cookie.Value, variant, message)
	}
}

// takeFlash returns and clears the request session's flash.
func (s *Server) takeFlash(r *http.Request) (variant, message string) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", ""
	}
	return s.sessions.TakeFlash(cookie.Value)
}

// crossSiteLogin reports whether a login POST is a genuine cross-site request —
// the CSRF defense for /login, which is pre-session and so carries no token. The
// attack it guards against is another site auto-submitting to /login, which the
// browser marks Sec-Fetch-Site: cross-site. same-origin, same-site (a request
// from the same registrable domain — routers are reached by bare IP/hostname and
// legitimately report this), and none are all allowed. For a client that sends
// no Sec-Fetch-Site (a non-browser, or an older one), fall back to comparing the
// Origin header's host to the request Host, refusing only a clear mismatch.
//
// Origin: null is explicitly allowed. Our own responses carry Referrer-Policy:
// no-referrer (VS-07), which makes some browsers — notably Safari, which here
// also omits Sec-Fetch-Site — send Origin: null on the login page's own POST.
// "null" is the opaque-origin marker, not a named cross-site origin, so it is not
// a mismatch; a real attacker's page still presents either Sec-Fetch-Site:
// cross-site or a named Origin whose host differs, both refused.
func crossSiteLogin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site":
		return true
	case "same-origin", "same-site", "none":
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		if u, err := url.Parse(origin); err != nil || !strings.EqualFold(u.Host, r.Host) {
			return true
		}
	}
	return false
}

func validCSRF(r *http.Request, want string) bool {
	if want == "" {
		return false
	}
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		// Keep file uploads off the router's heap; only ordinary form fields stay
		// in memory and the multipart package spills the archive to /tmp.
		err = r.ParseMultipartForm(1 << 20)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
		log.Printf("verso: parse CSRF form: %v", err)
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.PostForm.Get("_csrf")), []byte(want)) == 1
}

type loginData struct {
	CSS      template.CSS
	Error    string
	Version  string // the deployed Verso build ("dev" when un-stamped), shown in the hero so the running version is verifiable without signing in
	Firmware string // the OpenWrt release + revision, shown quietly in the hero
}

// loginFirmware reads the OpenWrt release + revision from /etc/openwrt_release
// (DISTRIB_DESCRIPTION, e.g. "OpenWrt 25.12.4 r32933-4ccb782af7") for the hero
// footer. Empty on any trouble, so the page falls back to a plain "OpenWrt".
func loginFirmware() string {
	data, err := os.ReadFile("/etc/openwrt_release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "DISTRIB_DESCRIPTION="); ok {
			return strings.Trim(strings.TrimSpace(v), "'\"")
		}
	}
	return ""
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
	// /login is pre-session, so it can carry no CSRF token; a same-origin check
	// is its CSRF defense against a cross-site auto-submit (VS-04).
	if crossSiteLogin(r) {
		s.renderLogin(w, http.StatusForbidden, "That request didn’t come from this page — reload and try again.")
		return
	}
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
	token, err := s.sessions.CreateWithMetadata(sid, username, clientIP(r), r.UserAgent())
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
	if err := s.page.ExecuteTemplate(&buf, "login.html.tmpl", loginData{CSS: s.css, Error: errMsg, Version: version.Version, Firmware: loginFirmware()}); err != nil {
		http.Error(w, "login page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

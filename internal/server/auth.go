// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	return p == "/login" || p == "/login/status" || p == "/healthz" || strings.HasPrefix(p, "/assets/")
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
			http.Redirect(w, r, loginRedirect(r), http.StatusSeeOther)
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
		// A page can read the uplink's live state in several sections; a
		// per-request memo shares one backend read so they cannot disagree.
		next.ServeHTTP(w, withWANMemo(r))
	})
}

// loginRedirect is where a request without a live session goes. A browser that
// presents a session cookie was signed in and is not any more — the sign-in ran
// out of time — so its login page carries the marker that explains the sign-out.
// A browser presenting no cookie was never signed in and is told nothing; a
// deliberate sign-out clears the cookie before it redirects, and so lands there
// too. Both POST and GET take this exit: once the session is gone the work is
// lost either way, and a redirect is the honest outcome.
func loginRedirect(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		return "/login?expired=1"
	}
	return "/login"
}

func (s *Server) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	if r.Method == http.MethodGet && r.Header.Get("X-Verso-Refresh") == "1" {
		return s.sessions.peek(cookie.Value)
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

// sessionAlive reports whether the request's session is still live without
// sliding its idle window — the check for a connection the browser holds open
// rather than a page a person asked for.
func (s *Server) sessionAlive(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.sessions.alive(cookie.Value)
}

// sessionExpiryStamp is the moment the request's session ends, RFC3339 in UTC.
// Every page stamps it so a page left open follows its own session out instead
// of learning about the sign-out from the next click. Empty without a session.
func (s *Server) sessionExpiryStamp(r *http.Request) string {
	sess, ok := s.currentSession(r)
	if !ok {
		return ""
	}
	return s.sessions.expiresAt(sess).UTC().Format(time.RFC3339)
}

// handleSessionState answers how long the request's session has left, in whole
// seconds, and whether activity can lengthen it. The page asks before it warns
// that the session is ending, since another tab may have kept it alive, and
// asks with the refresh header so the look itself is not activity; "Stay signed
// in" posts here, and that post is. Seconds rather than a timestamp, so the
// browser's clock never has to agree with the router's.
func (s *Server) handleSessionState(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, loginRedirect(r), http.StatusSeeOther)
		return
	}
	left := s.sessions.expiresAt(sess).Sub(s.sessions.now()).Round(time.Second)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Remaining  int  `json:"remaining"`
		Extendable bool `json:"extendable"`
	}{int(max(left, 0) / time.Second), s.sessions.extendable(sess)})
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
	if r.Method == http.MethodGet && r.Header.Get("X-Verso-Refresh") == "1" {
		return "", ""
	}
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

// loginData is everything the router is willing to say before it knows who is
// asking. The rule that picks what belongs here: a fact the box can state about
// itself or about the connection in front of it — its name, its release, how
// long it has been up, and the visitor's own address and subnet. No network
// inventory or configuration crosses this boundary, and no rpcd session is used.
type loginData struct {
	Lang           string // negotiated language for <html lang>, "en" when English
	CSS            template.CSS
	Error          string
	Notice         string // a statement about how the visitor got here, shown in a marigold notice
	Version        string // the deployed Verso build ("dev" when un-stamped), so the running version is verifiable without signing in
	Firmware       string // the distribution name and release
	Hostname       string // this board's name — the nameplate the whole rail hangs from
	Maker          string // who built the board
	Model          string // what they call it
	Status         loginStatus
	Username       string
	VisitorIP      string
	VisitorNetwork string // only the directly connected subnet containing this visitor
	Attempts       string // failed sign-ins standing against this visitor's address
	AttemptPolicy  string // the actual limiter policy, translated independently of the count
}

// loginFirmware is the release this board runs, named and versioned: "OpenWrt
// 25.12.4". Empty on any trouble, so the page falls back to a plain "OpenWrt".
func loginFirmware() string {
	data, err := os.ReadFile("/etc/openwrt_release")
	if err != nil {
		return ""
	}
	return releaseName(data)
}

// releaseName composes the distribution's name and version out of the two keys
// that carry them apart, so leaving the build revision off is a matter of not
// reading it rather than of cutting a formatted string back down. The revision
// identifies a build for a bug report; it tells a visitor nothing here. A build
// that names no release keeps whatever its description says, revision and all,
// rather than going silent about what it is running.
func releaseName(data []byte) string {
	release := releaseKeys(data)
	version := release["DISTRIB_RELEASE"]
	if version == "" {
		return release["DISTRIB_DESCRIPTION"]
	}
	name := release["DISTRIB_ID"]
	if name == "" {
		name = "OpenWrt"
	}
	return name + " " + version
}

// releaseKeys reads /etc/openwrt_release, a shell fragment of KEY='value'
// lines, into its keys with the quotes taken off.
func releaseKeys(data []byte) map[string]string {
	release := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		release[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "'\"")
	}
	return release
}

// loginUptime is how long this board has been running, in seconds. Zero on any
// trouble, which the page words as a fresh boot rather than hiding the row.
func loginUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	return procUptimeSeconds(data)
}

// procUptimeSeconds takes the first field of /proc/uptime — seconds since boot,
// with a fractional part — and truncates it.
func procUptimeSeconds(data []byte) int64 {
	first, _, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	secs, err := strconv.ParseFloat(first, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return int64(secs)
}

// attemptsNote states the failures standing against the visitor's address, so a
// run of guesses against this router is visible before anyone signs in. Empty
// at zero: a clean address says nothing rather than reassuring at every visit.
func attemptsNote(tr func(string) string, failures int) string {
	if failures <= 0 {
		return ""
	}
	// Catalogs use source-string keys. Spell out the small counts so a
	// translation can express dual and paucal forms as well as singular/plural.
	switch failures {
	case 1:
		return tr("1 failed sign-in from your address.")
	case 2:
		return tr("2 failed sign-ins from your address.")
	case 3:
		return tr("3 failed sign-ins from your address.")
	case 4:
		return tr("4 failed sign-ins from your address.")
	}
	return fmt.Sprintf(tr("%d failed sign-ins from your address."), failures)
}

// expiryNotice is the login page's statement for a visitor the middleware sent
// here: the sign-in ran out of time. It is a fact about the session, not a
// failure of this visit, so it reads plainly and wears the info tone. Empty
// unless the redirect marked the request.
func expiryNotice(r *http.Request) string {
	if r.URL.Query().Get("expired") != "1" {
		return ""
	}
	return "You were signed out after a period of inactivity."
}

// handleLoginForm serves the login page (redirecting an already-signed-in user
// home).
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentSession(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, r, http.StatusOK, "")
}

// handleLogin authenticates and, on success, stores the session server-side and
// hands the browser the opaque cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// /login is pre-session, so it can carry no CSRF token; a same-origin check
	// is its CSRF defense against a cross-site auto-submit (VS-04).
	if crossSiteLogin(r) {
		s.renderLogin(w, r, http.StatusForbidden, "That request didn’t come from this page — reload and try again.")
		return
	}
	key := clientIP(r)
	if !s.loginLimiter.allowed(key) {
		s.renderLogin(w, r, http.StatusTooManyRequests, "Too many attempts — wait a minute and try again.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, r, http.StatusOK, "Could not read the form.")
		return
	}
	username := r.PostForm.Get("username")

	sid, err := s.auth.Login(r.Context(), username, r.PostForm.Get("password"))
	if err != nil {
		// One generic message regardless of cause — do not reveal which of the
		// username or password was wrong. The detail is logged, not shown.
		s.loginLimiter.fail(key)
		log.Printf("verso: login failed for %q: %v", username, err)
		s.renderLogin(w, r, http.StatusOK, "Invalid username or password.")
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

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	// /login is pre-session, so the language is negotiated from Accept-Language
	// like any other request; the error copy is localized here at its one exit.
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	hostname, _ := os.Hostname()
	hw := board()
	username := "root"
	if r.PostForm != nil {
		username = r.PostForm.Get("username")
	}
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "login.html.tmpl", loginData{
		Lang: langAttr(lang), CSS: s.currentCSS(), Error: tr(errMsg), Notice: tr(expiryNotice(r)),
		Version: version.Version, Firmware: loginFirmware(),
		Hostname: hostname, Maker: hw.Maker, Model: hw.Model,
		Status: s.readLoginStatus(tr), Username: username,
		VisitorIP: clientIP(r), VisitorNetwork: visitorNetwork(clientIP(r)),
		Attempts:      attemptsNote(tr, s.loginLimiter.failures(clientIP(r))),
		AttemptPolicy: fmt.Sprintf(tr("After %d, this address waits a minute."), loginMaxFailures),
	}); err != nil {
		http.Error(w, "login page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// fakeBackend is a Backend seam double: no ubus/uci, no device needed. access and
// accessErr drive the plugin-write authorization gate (ADR-007) in tests.
type fakeBackend struct {
	si        openwrt.SystemInfo
	hn        string
	err       error
	access    bool
	accessErr error
}

func (f fakeBackend) SystemInfo(context.Context, string) (openwrt.SystemInfo, error) {
	return f.si, f.err
}

func (f fakeBackend) Hostname(context.Context, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.hn, nil
}

func (f fakeBackend) Access(context.Context, string, string, string, string) (bool, error) {
	return f.access, f.accessErr
}

// fakeTransport is the plugin-transport seam double (ADR-003/006): it returns a
// canned envelope or error and records the request the gateway forwarded, so the
// gateway is testable with no plugin process and no socket.
type fakeTransport struct {
	env        *plugin.Envelope
	err        error
	lastSocket string
	lastReq    plugin.Request
}

func (f *fakeTransport) Fetch(_ context.Context, socket string, req plugin.Request) (*plugin.Envelope, error) {
	f.lastSocket = socket
	f.lastReq = req
	return f.env, f.err
}

// fakeAuth / fakeSecurity are the auth seam doubles (ADR-003): no ubus, no shadow
// file, no device.
type fakeAuth struct {
	sid string
	err error
}

func (f fakeAuth) Login(context.Context, string, string) (string, error) { return f.sid, f.err }

type fakeSecurity struct{ hasPassword bool }

func (f fakeSecurity) RootHasPassword() bool { return f.hasPassword }

func newServerFull(t *testing.T, backend openwrt.Backend, tr plugin.Transport, manifests []plugin.Manifest, auth Authenticator, sec Security) *Server {
	t.Helper()
	r, err := widget.NewRenderer()
	if err != nil {
		t.Fatalf("widget.NewRenderer: %v", err)
	}
	s, err := New(r, backend, tr, manifests, auth, sec)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func newServerWith(t *testing.T, backend openwrt.Backend, tr plugin.Transport, manifests []plugin.Manifest) *Server {
	return newServerFull(t, backend, tr, manifests, fakeAuth{sid: "test-sid"}, fakeSecurity{hasPassword: true})
}

func newServer(t *testing.T, backend openwrt.Backend) *Server {
	return newServerWith(t, backend, &fakeTransport{}, nil)
}

func demoManifest() plugin.Manifest {
	return plugin.Manifest{
		ManifestVersion: 1, ID: "demo", Name: "Demo Plugin",
		Socket: "/run/verso/demo.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Apps", Label: "Demo", Path: "/"}},
	}
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	// Authenticate by default: mint a session and attach its cookie, so the
	// behavior tests exercise the page rather than the login redirect.
	token, err := srv.sessions.Create("test-sid", "root")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func postForm(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// postPlugin issues an authenticated, CSRF-valid POST to a plugin path — the
// path a real browser save takes — so the ACL gate (ADR-007) is exercised end to
// end rather than short-circuited by the auth or CSRF middleware.
func postPlugin(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	token, err := srv.sessions.Create("test-sid", "root")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess, _ := srv.sessions.get(token)
	if form == nil {
		form = url.Values{}
	}
	form.Set("_csrf", sess.csrf)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func demoACLManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "write"}}}
	return m
}

func TestHealthzReturnsOK(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{}), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok\n"; got != want {
		t.Fatalf("GET /healthz: body = %q, want %q", got, want)
	}
}

func TestIndexRendersRealData(t *testing.T) {
	backend := fakeBackend{
		hn: "verso-lab",
		si: openwrt.SystemInfo{
			Uptime: 3661,
			Load:   [3]int64{65536, 0, 0},
			Memory: openwrt.Memory{Total: 64883740672, Available: 37969338368},
		},
	}
	rec := get(t, newServer(t, backend), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"<table", "verso-lab", "1h 1m", "1.00", "GiB"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
}

func TestIndexDegradesWhenBackendFails(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{err: errors.New("ubus down")}), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200 (must degrade, not 500)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("GET /: expected 'unavailable' degradation")
	}
}

// TestPluginPageRendersSchema is the core question's first yes: a plugin's schema
// (fetched via the transport) renders through the shell's own widget renderer and
// chrome, and the request reached the plugin's socket intact.
func TestPluginPageRendersSchema(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo Page",
		Widget: json.RawMessage(`{"type":"card","title":"Hello","children":[{"type":"table","columns":["A"],"rows":[["1"]]}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Demo Page", "Hello", "<table", "<section"} {
		if !strings.Contains(body, want) {
			t.Errorf("plugin page missing %q", want)
		}
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("dialed socket = %q, want the manifest's", tr.lastSocket)
	}
	if tr.lastReq.Method != http.MethodGet || tr.lastReq.Path != "" {
		t.Errorf("forwarded request = %+v, want GET with empty sub-path", tr.lastReq)
	}
}

// TestPluginUnavailableWhenTransportFails is the crash-isolation contract, proven
// hermetically: a dead plugin renders "unavailable" in chrome, and the shell
// returns 200 — never 500, never a crash (ADR-006 §7).
func TestPluginUnavailableWhenTransportFails(t *testing.T) {
	tr := &fakeTransport{err: errors.New("dial unix: connection refused")}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a plugin must never 500 the shell)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected 'plugin unavailable' notice")
	}
}

// TestPluginValidation422Propagates: when a plugin returns 422 (a validation
// failure), the shell renders the error form AND passes 422 to the browser —
// the HTTP semantics stay honest, not flattened to 200 (ADR-006 §5).
func TestPluginValidation422Propagates(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusUnprocessableEntity,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"table","columns":["A"],"rows":[]}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 propagated from the plugin", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<section") {
		t.Errorf("a 422 must still render the (error) schema")
	}
}

// TestPluginBadSchemaIsUnavailable: a plugin returning garbage the widget set
// can't decode is contained exactly like a dead one.
func TestPluginBadSchemaIsUnavailable(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"nope"}`)}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected 'unavailable' for an undecodable schema")
	}
}

func TestPluginUnknownIDIs404(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)

	rec := get(t, s, "/plugins/ghost/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown plugin id", rec.Code)
	}
}

// TestPluginVersionMismatchDegrades: an unsupported schema_version is a notice,
// not a fetch and not a crash (ADR-006 §6). The transport must not be dialed.
func TestPluginVersionMismatchDegrades(t *testing.T) {
	m := demoManifest()
	m.SchemaVersion = 99
	tr := &fakeTransport{err: errors.New("must not be called")}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{m})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "newer Verso") {
		t.Errorf("expected version-mismatch notice, got: %s", rec.Body.String())
	}
	if tr.lastSocket != "" {
		t.Errorf("transport was dialed on a version mismatch; it must not be")
	}
}

// TestPluginWriteAllowedWhenSessionGranted: a POST whose session holds the
// plugin's declared write grant is dispatched to the plugin and its schema
// renders. The gate sits in front of the existing gateway (ADR-007).
func TestPluginWriteAllowedWhenSessionGranted(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved",
		Widget: json.RawMessage(`{"type":"card","title":"Saved","children":[{"type":"table","columns":["A"],"rows":[["1"]]}]}`),
	}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("granted write was not dispatched to the plugin (socket %q)", tr.lastSocket)
	}
	if got := url.Values(tr.lastReq.Form).Get("hostname"); got != "verso-lab" {
		t.Errorf("form not forwarded: hostname=%q", got)
	}
	if _, ok := tr.lastReq.Form["_csrf"]; ok {
		t.Errorf("the shell's _csrf token must not be forwarded to the plugin")
	}
}

// TestPluginWriteRefusedWhenSessionDenied: a POST whose session lacks the grant
// is refused by the shell with 403 and NEVER reaches the plugin — the shell is
// the enforcement point (ADR-007).
func TestPluginWriteRefusedWhenSessionDenied(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{access: false}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a denied write", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("a denied write must not reach the plugin; dialed %q", tr.lastSocket)
	}
	if !strings.Contains(rec.Body.String(), "permitted") {
		t.Errorf("expected a 'not permitted' notice")
	}
}

// TestPluginWriteRefusedWithoutDeclaredACL: fail closed — a plugin that declares
// no write scopes cannot receive a state-changing request, even for a session
// that would pass any check (ADR-007).
func TestPluginWriteRefusedWithoutDeclaredACL(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoManifest()}) // no ACL

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when the plugin declares no write scopes", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("must not reach a plugin with no declared write ACL; dialed %q", tr.lastSocket)
	}
}

// TestPluginWriteFailsClosedWhenACLCheckErrors: if rpcd can't be reached to
// authorize, the shell fails closed (503) rather than dispatching the write.
func TestPluginWriteFailsClosedWhenACLCheckErrors(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{accessErr: errors.New("rpcd down")}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the ACL check errors", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("must not dispatch when authorization is unknown; dialed %q", tr.lastSocket)
	}
}

// TestPluginGetNotGated: reads are not gated — a GET renders even with no write
// grant, so a read-only operator can still view the page (ADR-007 gates writes).
func TestPluginGetNotGated(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo",
		Widget: json.RawMessage(`{"type":"card","title":"Hi","children":[{"type":"table","columns":["A"],"rows":[["1"]]}]}`),
	}}
	s := newServerWith(t, fakeBackend{access: false}, tr, []plugin.Manifest{demoACLManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (GET must not be gated)", rec.Code)
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("GET should reach the plugin; dialed %q", tr.lastSocket)
	}
}

// TestNavListsPlugins: a discovered plugin appears in the shell nav with no shell
// change — the manifest drives it (ADR-006 §2).
func TestNavListsPlugins(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/")
	body := rec.Body.String()
	for _, want := range []string{"Apps", "Demo", `href="/plugins/demo/"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing %q", want)
		}
	}
}

// TestNavMultipleEntriesPerPlugin: one plugin can place several pages in the
// menu, each grouped under its section.
func TestNavMultipleEntriesPerPlugin(t *testing.T) {
	m := demoManifest()
	m.Nav = []plugin.NavEntry{
		{Section: "System", Label: "General", Path: "/"},
		{Section: "System", Label: "Time", Path: "/time"},
	}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})

	body := get(t, s, "/").Body.String()
	for _, want := range []string{"General", "Time", `href="/plugins/demo/"`, `href="/plugins/demo/time"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing %q", want)
		}
	}
}

// TestHostGuardRejectsUnknownHost: a request with a Host outside the allowlist
// is refused (VS-01, DNS-rebinding defense).
func TestHostGuardRejectsUnknownHost(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	srv.SetAllowedHosts([]string{"verso.lan", "127.0.0.1"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown host: status = %d, want 400", rec.Code)
	}
}

func TestHostGuardAllowsConfiguredHost(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	srv.SetAllowedHosts([]string{"verso.lan"})
	token, _ := srv.sessions.Create("sid", "root")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "verso.lan:8080" // port is stripped before the check
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("allowed host: status = %d, want 200", rec.Code)
	}
}

// TestSecurityHeaders: defensive headers are set on every response (VS-07).
func TestSecurityHeaders(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{}), "/")
	h := rec.Header()
	if !strings.Contains(h.Get("Content-Security-Policy"), "script-src 'none'") {
		t.Errorf("CSP missing or not strict: %q", h.Get("Content-Security-Policy"))
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", h.Get("X-Frame-Options"))
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options missing")
	}
}

// TestUnauthenticatedRedirectsToLogin: the middleware gates every non-public page.
func TestUnauthenticatedRedirectsToLogin(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no cookie
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestLoginPageIsPublic(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sign in") {
		t.Errorf("login page missing the form")
	}
}

func TestLoginSuccessSetsHttpOnlyCookieAndRedirects(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{sid: "rpcd-sid"}, fakeSecurity{hasPassword: true})

	rec := postForm(t, srv, "/login", url.Values{"username": {"root"}, "password": {"pw"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie || cookies[0].Value == "" {
		t.Fatalf("no session cookie set: %v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Errorf("session cookie must be HttpOnly")
	}
}

func TestLoginFailureShowsErrorAndNoCookie(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{err: errors.New("denied")}, fakeSecurity{hasPassword: true})

	rec := postForm(t, srv, "/login", url.Values{"username": {"root"}, "password": {"bad"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render)", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("a failed login must not set a session cookie")
	}
	if !strings.Contains(rec.Body.String(), "Invalid") {
		t.Errorf("expected an error message")
	}
}

// TestLoginThrottled: repeated failures from one client lock further attempts
// with 429 (VS-06).
func TestLoginThrottled(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil,
		fakeAuth{err: errors.New("denied")}, fakeSecurity{hasPassword: true})
	bad := url.Values{"username": {"root"}, "password": {"wrong"}}

	for i := 0; i < loginMaxFailures; i++ {
		if rec := postForm(t, srv, "/login", bad); rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", i, rec.Code)
		}
	}
	if rec := postForm(t, srv, "/login", bad); rec.Code != http.StatusTooManyRequests {
		t.Errorf("after %d failures: status = %d, want 429", loginMaxFailures, rec.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token, _ := srv.sessions.Create("sid", "root")
	sess, _ := srv.sessions.get(token)

	form := url.Values{"_csrf": {sess.csrf}}
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if _, ok := srv.sessions.get(token); ok {
		t.Errorf("session was not destroyed")
	}
}

// TestCSRFRejectsPostWithoutToken: a state-changing request lacking the session
// CSRF token is refused (VS-04).
func TestCSRFRejectsPostWithoutToken(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token, _ := srv.sessions.Create("sid", "root")

	req := httptest.NewRequest(http.MethodPost, "/logout", nil) // no _csrf
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token: status = %d, want 403", rec.Code)
	}
}

// TestNoPasswordBanner: the in-app warning shows only when root has no password.
func TestNoPasswordBanner(t *testing.T) {
	warn := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{sid: "s"}, fakeSecurity{hasPassword: false})
	if !strings.Contains(get(t, warn, "/").Body.String(), "No root password") {
		t.Errorf("expected the no-password banner")
	}
	safe := newServer(t, fakeBackend{}) // hasPassword true
	if strings.Contains(get(t, safe, "/").Body.String(), "No root password") {
		t.Errorf("must not warn when a password is set")
	}
}

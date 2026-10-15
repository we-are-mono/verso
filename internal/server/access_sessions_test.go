// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A session is where it signed in from and when it was last seen: the browser
// reads under the address, and when it started under when it was last seen,
// so a full IPv6 address and a way to revoke fit the form's measure. Revoking
// is an icon on the row that asks first.
func TestSessionsFoldIntoSourceAndLastSeen(t *testing.T) {
	table := accessSessionsTable([]accessSession{
		{ID: "a", Address: "10.0.0.232", Browser: "Firefox · Linux", Since: "since today, 16:43", LastActive: "Now", Current: true},
		{ID: "b", Address: "fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b", Browser: "Safari · iPhone", Since: "since 22 Sep, 09:12", LastActive: "3 min ago"},
	}, identity)

	var labels, kinds []string
	for _, c := range table.Columns {
		labels, kinds = append(labels, c.Label), append(kinds, c.Kind)
	}
	if len(labels) != 3 || labels[0] != "Source" || labels[1] != "Last seen" || labels[2] != "" ||
		kinds[0] != "reference" || kinds[1] != "text" || kinds[2] != "actions" {
		t.Fatalf("columns = %v %v, want Source, Last seen and the row's act", labels, kinds)
	}

	current, other := table.Rows[0].Cells, table.Rows[1].Cells
	if src := other[0]; src.Text != "fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b" || src.Detail != "Safari · iPhone" || src.Sub != "" {
		t.Errorf("source = %+v, want the address over its browser", src)
	}
	if seen := other[1]; seen.Text != "3 min ago" || seen.Detail != "since 22 Sep, 09:12" {
		t.Errorf("last seen = %+v, want the age over when it started", seen)
	}
	acts := other[2].Actions
	if len(acts) != 1 || acts[0].Icon != "log-out" || acts[0].Title != "Revoke session" ||
		acts[0].Name != "_action" || acts[0].Value != "end-session:b" || other[2].Confirm == "" {
		t.Errorf("revoke = %+v (confirm %q), want one icon act that asks first", acts, other[2].Confirm)
	}

	if current[0].Tag != "this browser" {
		t.Errorf("this browser is marked: %+v", current[0])
	}
	// This browser revokes itself with the same act, which signs it out; the
	// question says so in its own words.
	own := current[2]
	if len(own.Actions) != 1 || own.Actions[0].Value != "end-session:a" || own.Actions[0].Icon != "log-out" ||
		own.Confirm == "" || own.Confirm == other[2].Confirm {
		t.Errorf("own revoke = %+v, want the same act asking in its own words", own)
	}
}

// Revoking this browser's own session is logging out: the session ends, the
// cookie goes, and the browser lands on sign-in.
func TestRevokingOwnSessionLogsOut(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	token := s.sessions.CreateWithMetadata("sid-current", "root", "10.0.0.232", "Firefox/142.0")
	current, _ := s.sessions.get(token)
	form := url.Values{"_csrf": {current.csrf}, "_action": {"end-session:" + current.id}}
	post := httptest.NewRequest(http.MethodPost, "/system/access", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, post)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("revoking own session = %d → %q, want 303 → /login", rec.Code, rec.Header().Get("Location"))
	}
	if _, ok := s.sessions.get(token); ok {
		t.Error("own session is still live")
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the session cookie was not cleared")
	}
}

// Another account's sessions are rpcd's to manage: a session sees and ends
// its own account's sessions, and another account's only while rpcd grants
// it ubus session list and session destroy, as rpcd's own session object
// would ask. Root's '*' holds both, through verso-admin
// (TestRootIsGrantedEveryShellProbe), and sees and ends every session as
// before.
func TestAnotherAccountsSessionsNeedRPCDsSessionGrants(t *testing.T) {
	s := passwordServer(t, fakeBackend{grant: grantAllBut("ubus session list", "ubus session destroy")})
	rootToken := s.sessions.CreateWithMetadata("sid-root", "root", "10.0.0.2", "Firefox/142.0")
	guestToken := s.sessions.CreateWithMetadata("sid-guest", "guest", "10.0.0.50", "Firefox/142.0")
	phoneToken := s.sessions.CreateWithMetadata("sid-guest-phone", "guest", "10.0.0.51", "Mozilla/5.0 (iPhone) Version/18.0 Safari/605.1")
	root, _ := s.sessions.get(rootToken)
	guest, _ := s.sessions.get(guestToken)
	phone, _ := s.sessions.get(phoneToken)
	access := func(token string) string {
		req := httptest.NewRequest(http.MethodGet, "/system/access", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Body.String()
	}
	end := func(token, csrf, id string) int {
		form := url.Values{"_csrf": {csrf}, "_action": {"end-session:" + id}}
		post := httptest.NewRequest(http.MethodPost, "/system/access", strings.NewReader(form.Encode()))
		post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		post.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, post)
		return rec.Code
	}

	body := access(guestToken)
	if strings.Contains(body, "10.0.0.2<") || strings.Contains(body, "end-session:"+root.id) {
		t.Error("a session without rpcd's session list grant sees another account's session")
	}
	if !strings.Contains(body, "10.0.0.51") || !strings.Contains(body, "end-session:"+phone.id) {
		t.Error("a session sees its own account's other sessions")
	}
	if code := end(guestToken, guest.csrf, root.id); code != http.StatusBadRequest {
		t.Errorf("ending another account's session without the grant = %d, want 400", code)
	}
	if _, alive := s.sessions.get(rootToken); !alive {
		t.Fatal("a session without rpcd's session destroy grant ended root's session")
	}
	if code := end(guestToken, guest.csrf, phone.id); code != http.StatusSeeOther {
		t.Errorf("ending its own account's other session = %d, want 303", code)
	}

	b := s.backend.(fakeBackend)
	b.grant = nil
	b.access = true
	s.backend = b
	if body := access(rootToken); !strings.Contains(body, "10.0.0.50") || !strings.Contains(body, "end-session:"+guest.id) {
		t.Error("root sees every account's sessions")
	}
	if code := end(rootToken, root.csrf, guest.id); code != http.StatusSeeOther {
		t.Errorf("root ending another account's session = %d, want 303", code)
	}
	if _, alive := s.sessions.get(guestToken); alive {
		t.Error("root's end left the session live")
	}
}

// When a session started is said as "since", today by the clock alone and any
// other day by its date.
func TestSessionSince(t *testing.T) {
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		created time.Time
		want    string
	}{
		{time.Date(2026, 9, 23, 16, 43, 0, 0, time.Local), "since today, 16:43"},
		{time.Date(2026, 9, 22, 9, 12, 0, 0, time.Local), "since 22 Sep, 09:12"},
	} {
		if got := sessionSince(identity, tc.created, now); got != tc.want {
			t.Errorf("sessionSince(%v) = %q, want %q", tc.created, got, tc.want)
		}
	}
}

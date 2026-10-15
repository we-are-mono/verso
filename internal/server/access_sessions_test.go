// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
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

// Changing a password ends every other session of the account whose password
// it was, on either route the form posts to: a browser signed in with the old
// password lands on sign-in and its rpcd sid is torn down. The browser that
// made the change stays signed in, and another account's sessions stand.
func TestPasswordChangeEndsTheAccountsOtherSessions(t *testing.T) {
	for _, path := range []string{"/system/access", "/system/password"} {
		t.Run(path, func(t *testing.T) {
			s := passwordServer(t, fakeBackend{setPassword: func(context.Context, string, string, string) error { return nil }})
			k := &fakeKeeper{alive: true}
			s.sessions.keeper = k
			changing := s.sessions.CreateWithMetadata("sid-changing", "root", "10.0.0.2", "Firefox/142.0")
			stale := s.sessions.CreateWithMetadata("sid-stale", "root", "10.0.0.66", "Firefox/142.0")
			guest := s.sessions.CreateWithMetadata("sid-guest", "guest", "10.0.0.50", "Firefox/142.0")
			sess, _ := s.sessions.get(changing)
			send := func(token, method, path string, form url.Values) *httptest.ResponseRecorder {
				var req *http.Request
				if form != nil {
					req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				} else {
					req = httptest.NewRequest(method, path, nil)
				}
				req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				return rec
			}

			rec := send(changing, http.MethodPost, path, url.Values{
				"_csrf": {sess.csrf}, "current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"correct-horse"},
			})
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/access" {
				t.Fatalf("password change = %d → %q, want 303 → /system/access", rec.Code, rec.Header().Get("Location"))
			}

			if rec := send(stale, http.MethodGet, "/system/access", nil); rec.Code != http.StatusSeeOther ||
				!strings.HasPrefix(rec.Header().Get("Location"), "/login") {
				t.Errorf("the account's other session = %d → %q, want 303 → sign-in", rec.Code, rec.Header().Get("Location"))
			}
			if rec := send(changing, http.MethodGet, "/system/access", nil); rec.Code != http.StatusOK {
				t.Errorf("the session that changed the password = %d, want 200", rec.Code)
			}
			if rec := send(guest, http.MethodGet, "/system/access", nil); rec.Code != http.StatusOK {
				t.Errorf("another account's session = %d, want 200", rec.Code)
			}
			s.sessions.renewTick(context.Background())
			if _, destroyed := k.counts(); len(destroyed) != 1 || destroyed[0] != "sid-stale" {
				t.Errorf("destroyed sids = %v, want the other session's sid-stale alone", destroyed)
			}
		})
	}
}

// A password change that fails ends no session: the old password still holds.
func TestFailedPasswordChangeEndsNoSession(t *testing.T) {
	s := passwordServer(t, fakeBackend{setPassword: func(context.Context, string, string, string) error {
		return errors.New("passwd failed")
	}})
	other := s.sessions.CreateWithMetadata("sid-other", "root", "10.0.0.66", "Firefox/142.0")
	rec := postPlugin(t, s, "/system/access", url.Values{
		"current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"correct-horse"},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failed change = %d, want 500", rec.Code)
	}
	if !s.sessions.alive(other) {
		t.Error("a failed password change ended another session")
	}
}

// A session ended between the sign-in check and the change, by another browser
// changing the password say, changes nothing: the browser goes to sign in.
func TestPasswordChangeFromAnEndedSessionChangesNothing(t *testing.T) {
	changed := false
	s := passwordServer(t, fakeBackend{setPassword: func(context.Context, string, string, string) error {
		changed = true
		return nil
	}})
	other := s.sessions.CreateWithMetadata("sid-other", "root", "10.0.0.66", "Firefox/142.0")
	form := url.Values{"current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"correct-horse"}}
	req := httptest.NewRequest(http.MethodPost, "/system/access", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "ended"})
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("change from an ended session = %d → %q, want 303 → sign-in", rec.Code, rec.Header().Get("Location"))
	}
	if changed {
		t.Error("a session that had ended changed the password")
	}
	if _, ok := s.sessions.get(other); !ok {
		t.Error("a session that had ended ended another")
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

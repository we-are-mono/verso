// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// sessionState is what GET and POST /session answer: how long the session has
// left and whether staying signed in can lengthen it.
type sessionState struct {
	Remaining  int  `json:"remaining"`
	Extendable bool `json:"extendable"`
}

func readSessionState(t *testing.T, rec *httptest.ResponseRecorder) sessionState {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q, want JSON", ct)
	}
	var st sessionState
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
	return st
}

// TestSessionStateLooksWithoutTouching: the page asks how long its session has
// left before it warns, because another tab may have kept it alive. Asking is
// not activity, so the answer does not slide the idle window it reports.
func TestSessionStateLooksWithoutTouching(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	srv.sessions = newSessionsClock(clk.now)
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	clk.advance(29 * time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	req.Header.Set("X-Verso-Refresh", "1")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	st := readSessionState(t, rec)
	if st.Remaining != 60 || !st.Extendable {
		t.Errorf("state = %+v, want 60 s left and extendable", st)
	}
	clk.advance(61 * time.Second)
	if _, ok := srv.sessions.peek(token); ok {
		t.Errorf("looking at the session must not have kept it alive")
	}
}

// TestStayingSignedInSlidesTheSession: "Stay signed in" posts to /session, and
// that post is activity: the idle window starts again from now.
func TestStayingSignedInSlidesTheSession(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	srv.sessions = newSessionsClock(clk.now)
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	sess, _ := srv.sessions.peek(token)
	clk.advance(29 * time.Minute)

	form := url.Values{"_csrf": {sess.csrf}}
	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	st := readSessionState(t, rec)
	if want := int(sessionIdleTimeout / time.Second); st.Remaining != want || !st.Extendable {
		t.Errorf("state = %+v, want the whole idle window (%d s) again", st, want)
	}
}

// TestSessionAtItsCapCannotBeExtended: the absolute cap is not an idle window;
// near it, staying signed in buys nothing, and the answer says so.
func TestSessionAtItsCapCannotBeExtended(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	srv.sessions = newSessionsClock(clk.now)
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	sess, _ := srv.sessions.peek(token)
	// Kept busy until a minute short of the cap.
	for elapsed := time.Duration(0); elapsed < sessionAbsoluteTimeout-time.Minute; elapsed += 20 * time.Minute {
		clk.advance(20 * time.Minute)
		srv.sessions.get(token)
	}
	clk.t = time.Unix(1_000_000, 0).Add(sessionAbsoluteTimeout - time.Minute)
	srv.sessions.get(token)

	form := url.Values{"_csrf": {sess.csrf}}
	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	st := readSessionState(t, rec)
	if st.Remaining != 60 || st.Extendable {
		t.Errorf("state = %+v, want 60 s left and not extendable", st)
	}
}

// TestPageCarriesTheSessionWarning: every signed-in page carries the warning
// the page raises before its session ends: an alert dialog naming the time
// left, the way to stay, and the way out.
func TestPageCarriesTheSessionWarning(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{}), "/").Body.String()
	for _, want := range []string{
		`id="verso-session-ending"`, `role="alertdialog"`, `aria-modal="true"`,
		`aria-labelledby="verso-session-ending-title"`, `aria-describedby="verso-session-ending-body"`,
		"data-verso-session-countdown",
		"data-verso-session-stay", "Stay signed in",
		`action="/logout"`, "Sign out",
		"Staged changes are kept. Anything typed but not saved will be lost.",
		// The cap's variant: nothing to extend, only to acknowledge — and the
		// acknowledgement says what it lets you do, not a bare "OK".
		"data-verso-session-capped", "Sessions last at most 12 hours.", "Keep working",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing the session warning's %q", want)
		}
	}
	if strings.Contains(body, ">OK</button>") {
		t.Error("no button says a bare OK")
	}
}

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
)

// passwordServer builds a shell whose password write is the given backend fake.
// The privileged write goes through the Backend (rpcd), not the shell (ADR-007).
func passwordServer(t *testing.T, backend fakeBackend) *Server {
	t.Helper()
	return newServerFull(t, backend, &fakeTransport{}, nil, fakeAuth{sid: "test-sid"})
}

func TestPasswordFormRenders(t *testing.T) {
	rec := get(t, passwordServer(t, fakeBackend{}), "/system/password")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="current_password"`, `name="password"`, `name="confirm"`, `type="password"`,
		"Router password", "New password", "Repeat new password", "Change password", "Signed in now", "this browser",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("form missing %q", want)
		}
	}
	// A GET form must never carry a value for a password input.
	if strings.Contains(body, `type="password"`) && strings.Contains(body, `name="password" value=`) {
		t.Error("password input must not reflect a value")
	}
}

// The password form holds Change password until something is typed, and names
// the account it changes for the browser's password manager, which otherwise
// cannot tell whose new password it is saving.
func TestAccessPasswordFormWaitsAndNamesItsAccount(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{}), "/system/access").Body.String()
	form := body[strings.Index(body, `action="/system/access"`):]
	form = form[:strings.Index(form, "</form>")]
	for _, want := range []string{"data-verso-dirty-form", `autocomplete="username"`} {
		if !strings.Contains(form, want) {
			t.Errorf("password form missing %q:\n%s", want, form)
		}
	}
}

func TestPasswordlessAccessOmitsCurrentPasswordAndShowsNoStagedChip(t *testing.T) {
	body := get(t, passwordServer(t, fakeBackend{rootNoPassword: true}), "/system/access").Body.String()
	for _, want := range []string{"No administrator password is set.", "Set password", "New password", "Repeat new password"} {
		if !strings.Contains(body, want) {
			t.Errorf("passwordless Access missing %q", want)
		}
	}
	if strings.Contains(body, `name="current_password"`) {
		t.Error("passwordless Access must not ask for a current password")
	}
	if !strings.Contains(body, stagedChipHidden) {
		t.Error("the immediate Access form stages nothing, so the chip stays hidden")
	}
}

func TestPasswordRejectsIncorrectCurrentPassword(t *testing.T) {
	called := false
	s := newServerFull(t, fakeBackend{setPassword: func(context.Context, string, string, string) error {
		called = true
		return nil
	}}, &fakeTransport{}, nil, fakeAuth{sid: "test-sid", verifyErr: errors.New("denied")})
	form := url.Values{"current_password": {"wrong"}, "password": {"correct-horse"}, "confirm": {"correct-horse"}}
	rec := postPlugin(t, s, "/system/access", form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called {
		t.Error("SetPassword must not run when the current password is wrong")
	}
	if !strings.Contains(rec.Body.String(), "Current password is incorrect.") {
		t.Error("current-password error not shown")
	}
}

func TestAccessListsAndEndsOtherSessionWithoutExposingBearer(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	currentToken := s.sessions.CreateWithMetadata("sid-current", "root", "10.0.0.232", "Mozilla/5.0 (X11; Linux x86_64) Firefox/142.0")
	otherToken := s.sessions.CreateWithMetadata("sid-other", "root", "10.0.10.117", "Mozilla/5.0 (iPhone) Version/18.0 Safari/605.1")
	current, _ := s.sessions.get(currentToken)
	other, _ := s.sessions.get(otherToken)

	req := httptest.NewRequest(http.MethodGet, "/system/access", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: currentToken})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Firefox · Linux", "Safari · iPhone", "10.0.0.232", "10.0.10.117", "this browser", "Revoke", "end-session:" + other.id} {
		if !strings.Contains(body, want) {
			t.Errorf("sessions table missing %q", want)
		}
	}
	if strings.Contains(body, currentToken) || strings.Contains(body, otherToken) {
		t.Error("a session bearer token leaked into the Access page")
	}

	form := url.Values{"_csrf": {current.csrf}, "_action": {"end-session:" + other.id}}
	post := httptest.NewRequest(http.MethodPost, "/system/access", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(&http.Cookie{Name: sessionCookie, Value: currentToken})
	ended := httptest.NewRecorder()
	s.Handler().ServeHTTP(ended, post)
	if ended.Code != http.StatusSeeOther {
		t.Fatalf("end session status = %d, want 303", ended.Code)
	}
	if _, ok := s.sessions.get(otherToken); ok {
		t.Error("other session is still live")
	}
	if _, ok := s.sessions.get(currentToken); !ok {
		t.Error("ending another session destroyed the current session")
	}
}

func TestPasswordChangeSucceeds(t *testing.T) {
	var gotSID, gotUser, gotPass string
	called := false
	backend := fakeBackend{setPassword: func(_ context.Context, sid, u, p string) error {
		called, gotSID, gotUser, gotPass = true, sid, u, p
		return nil
	}}
	form := url.Values{"current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"correct-horse"}}
	do := sameSession(t, passwordServer(t, backend))
	rec := do(http.MethodPost, "/system/password", form)

	// Access is read again, so a reload never sends the password twice.
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/access" {
		t.Fatalf("status = %d → %q, want 303 to Access", rec.Code, rec.Header().Get("Location"))
	}
	if !called {
		t.Fatal("SetPassword was not called")
	}
	if gotSID != "test-sid" {
		t.Errorf("sid = %q, want test-sid (the session sid carried to rpcd)", gotSID)
	}
	if gotUser != "root" {
		t.Errorf("username = %q, want root (the session user)", gotUser)
	}
	if gotPass != "correct-horse" {
		t.Errorf("password = %q, want correct-horse", gotPass)
	}
	// How it went arrives with Access, for the notification to say — never
	// as a line in the content.
	body := do(http.MethodGet, "/system/access", nil).Body.String()
	if !strings.Contains(arrival(body), "Password updated.") {
		t.Error("Access did not arrive with the outcome")
	}
	if inContent(body, "Password updated.") {
		t.Error("the outcome is drawn in the content")
	}
}

func TestPasswordRejectsMismatch(t *testing.T) {
	called := false
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { called = true; return nil }}
	form := url.Values{"current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"battery-staple"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called {
		t.Error("SetPassword must not be called on a mismatch")
	}
	if !strings.Contains(rec.Body.String(), "Passwords do not match.") {
		t.Error("mismatch error not shown")
	}
}

func TestPasswordRejectsShort(t *testing.T) {
	called := false
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { called = true; return nil }}
	form := url.Values{"current_password": {"old-password"}, "password": {"short"}, "confirm": {"short"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called {
		t.Error("SetPassword must not be called on a too-short password")
	}
	if !strings.Contains(rec.Body.String(), "Use at least 8 characters.") {
		t.Error("length error not shown")
	}
}

func TestPasswordRejectsEmpty(t *testing.T) {
	called := false
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { called = true; return nil }}
	form := url.Values{"current_password": {"old-password"}, "password": {""}, "confirm": {""}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called {
		t.Error("SetPassword must not be called on an empty password")
	}
	if !strings.Contains(rec.Body.String(), "Enter a new password.") {
		t.Error("empty error not shown")
	}
}

func TestPasswordBackendFailureIsContained(t *testing.T) {
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error {
		return context.DeadlineExceeded
	}}
	form := url.Values{"current_password": {"old-password"}, "password": {"correct-horse"}, "confirm": {"correct-horse"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "be changed just now") {
		t.Error("backend-failure notice not shown")
	}
	// Still the styled form, not a bare error string.
	if !strings.Contains(body, "Change password") {
		t.Error("form should re-render on backend failure")
	}
}

// A submitted password must never appear in the re-rendered HTML — not in a value
// attribute, not anywhere — even on a validation error that re-renders the form.
func TestPasswordNeverEchoed(t *testing.T) {
	secret := "unique-secret-42x"
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { return nil }}
	form := url.Values{"current_password": {"old-password"}, "password": {secret}, "confirm": {"different-99y"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if strings.Contains(rec.Body.String(), secret) {
		t.Error("submitted password was echoed into the response")
	}
}

// The password page is reachable in the nav even with zero plugins, so a fresh
// (passwordless) device can set its first password.
func TestPasswordLinkAlwaysInNav(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	if row := railRow(t, sidebar(s, "/"), "System"); !row.Opens {
		t.Fatalf("System row = %+v, want it to open into its pages", row)
	}
	found := false
	for _, page := range s.sectionPages("System", "/") {
		if page.Href == "/system/access" && page.Label == "Access" {
			found = true
		}
	}
	if !found {
		t.Error("Access page missing from System pages")
	}
}

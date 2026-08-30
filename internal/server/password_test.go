// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
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
		`name="password"`, `name="confirm"`, `type="password"`,
		"New password", "Confirm new password", "Save password",
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

func TestPasswordChangeSucceeds(t *testing.T) {
	var gotSID, gotUser, gotPass string
	called := false
	backend := fakeBackend{setPassword: func(_ context.Context, sid, u, p string) error {
		called, gotSID, gotUser, gotPass = true, sid, u, p
		return nil
	}}
	form := url.Values{"password": {"correct-horse"}, "confirm": {"correct-horse"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
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
	if !strings.Contains(rec.Body.String(), "Password updated.") {
		t.Error("success message not shown")
	}
}

func TestPasswordRejectsMismatch(t *testing.T) {
	called := false
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { called = true; return nil }}
	form := url.Values{"password": {"correct-horse"}, "confirm": {"battery-staple"}}
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
	form := url.Values{"password": {"short"}, "confirm": {"short"}}
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
	form := url.Values{"password": {""}, "confirm": {""}}
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
	form := url.Values{"password": {"correct-horse"}, "confirm": {"correct-horse"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "be changed just now") {
		t.Error("backend-failure notice not shown")
	}
	// Still the styled form, not a bare error string.
	if !strings.Contains(body, "Save password") {
		t.Error("form should re-render on backend failure")
	}
}

// A submitted password must never appear in the re-rendered HTML — not in a value
// attribute, not anywhere — even on a validation error that re-renders the form.
func TestPasswordNeverEchoed(t *testing.T) {
	secret := "unique-secret-42x"
	backend := fakeBackend{setPassword: func(context.Context, string, string, string) error { return nil }}
	form := url.Values{"password": {secret}, "confirm": {"different-99y"}}
	rec := postPlugin(t, passwordServer(t, backend), "/system/password", form)

	if strings.Contains(rec.Body.String(), secret) {
		t.Error("submitted password was echoed into the response")
	}
}

// The password page is reachable in the nav even with zero plugins, so a fresh
// (passwordless) device can set its first password.
func TestPasswordLinkAlwaysInNav(t *testing.T) {
	sections := passwordServer(t, fakeBackend{}).buildNav("/")
	var system *navSection
	for i := range sections {
		if sections[i].Title == "System" {
			system = &sections[i]
		}
	}
	if system == nil {
		t.Fatal("System section missing from nav")
	}
	found := false
	for _, l := range system.Links {
		if l.Href == "/system/password" && l.Label == "Password" {
			found = true
		}
	}
	if !found {
		t.Error("Password link missing from System section")
	}
}

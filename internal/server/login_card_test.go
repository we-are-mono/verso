// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// The sign-in page is one card built from the shell's own widgets, so it reads
// as every other card does: the card's head, the form's rows and its act as
// the ledger's last row, then the router's public facts as a part of the card.
func TestLoginPageIsTheSharedSignInCard(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<section class="verso-card" data-verso-section="card">`,
		`<h2 class="verso-card__title">Sign in</h2>`,
		`<form method="post" action="/login" data-verso-questions="short" class="verso-form verso-rhythm">`,
		`<label for="username" id="username-label" class="verso-field__name">Username</label>`,
		`<label for="password" id="password-label" class="verso-field__name">Password</label>`,
		`type="password" autocomplete="current-password" autofocus`,
		`autocomplete="username" autocapitalize="none" spellcheck="false"`,
		`<div data-verso-form-actions class="verso-form__acts" data-verso-close="flush">`,
		`<h3 class="verso-section__part-title">This router</h3>`,
		`<dl class="verso-facts" data-verso-style="divided" data-verso-align="left">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the sign-in card is missing %s", want)
		}
	}
	// A takeover that holds a card stands it on Quiet Sand, as every page of
	// cards does, so the card's Paper reads as a card.
	if !strings.Contains(body, `<body class="verso-takeover" data-verso-surface="cards">`) {
		t.Error("the sign-in card does not stand on a surface of cards")
	}
	sheet, err := os.ReadFile("assets/blocks/takeover.css")
	if err != nil {
		t.Fatal(err)
	}
	if ground := cssBlock(t, string(sheet), `.verso-takeover[data-verso-surface="cards"] {`); !strings.Contains(ground, "background-color: var(--color-quiet);") {
		t.Errorf("a takeover of cards is not on Quiet Sand:\n%s", ground)
	}
	// The card is as narrow as its form of names allows: a fact list's 96px
	// label column, the gap and the answer column inside its gutters, 440px,
	// so the answers start where the facts' values do.
	login, err := os.ReadFile("assets/blocks/login.css")
	if err != nil {
		t.Fatal(err)
	}
	if measure := cssBlock(t, string(login), ".verso-login {"); !strings.Contains(measure, "max-width: calc(var(--spacing) * 110);") {
		t.Errorf("the sign-in card is not 440px:\n%s", measure)
	}
	// The page's own outline: the router is the page, signing in its card.
	if !strings.Contains(body, `<h1 class="sr-only">`) {
		t.Error("the sign-in page has no h1")
	}
	// The live refresh reads its clock and words off the card's frame.
	for _, hook := range []string{`data-login`, `data-unix="`, `data-offset="`, `data-busy-label="Signing in…"`, `<template data-verso-button-waiting>`} {
		if !strings.Contains(body, hook) {
			t.Errorf("the sign-in page is missing the live refresh's %s", hook)
		}
	}
}

// The Internet row says the state in words and in its packet, hung in the
// facts' gutter: online, offline, or unknown when the router cannot say.
func TestLoginInternetRowSaysTheState(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	for _, tc := range []struct {
		name  string
		up    bool
		err   error
		words string
		tone  string
	}{
		{"online", true, nil, "Online", "success"},
		{"offline", false, nil, "Offline", "danger"},
		{"unknown", false, errors.New("unavailable"), "Unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv.loginInternet = func() (bool, error) { return tc.up, tc.err }
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
			body := rec.Body.String()
			row := `<dt class="verso-facts__label">Internet</dt>`
			if !strings.Contains(body, row) {
				t.Fatalf("no Internet row")
			}
			mark := `<span class="verso-mark verso-facts__mark" data-verso-tone="` + tc.tone + `" data-verso-prop-dot="internet" aria-hidden="true"></span>`
			if !strings.Contains(body, mark) {
				t.Errorf("the Internet row's packet is not %s", tc.tone)
			}
			if !strings.Contains(body, `<span data-verso-prop="internet" class="verso-facts__text">`+tc.words+`</span>`) {
				t.Errorf("the Internet row does not say %q", tc.words)
			}
		})
	}
}

// A refusal stands where its cause is. Wrong credentials are the password's
// answer, under it. A refused request — another site's form, the attempt
// limit — is no fault of what was typed: it opens the form, and no field is
// marked.
func TestLoginRefusalsStandWhereTheirCauseIs(t *testing.T) {
	const notice = `<div class="verso-notice mb-6" data-verso-tone="danger">`
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{err: errors.New("denied")})
	bad := url.Values{"username": {"root"}, "password": {"wrong"}}

	wrong := postForm(t, srv, "/login", bad).Body.String()
	if !strings.Contains(wrong, `<span id="password-error" data-verso-error class="verso-refusal">`) {
		t.Error("wrong credentials are not refused under the password")
	}
	// The failed sign-ins standing against the address are worth a look
	// before anything is typed: the marigold notice opening the card, over
	// Username, and no note beside the act.
	const warning = `<div data-verso-callout class="verso-notice" data-verso-tone="warning">`
	before, after, found := strings.Cut(wrong, warning)
	if !found || !strings.Contains(after, "1 failed sign-in from your address. After 5, this address waits a minute.") {
		t.Error("the failed sign-ins are not the card's marigold notice")
	}
	if strings.Contains(before, `id="username"`) {
		t.Error("the marigold notice does not stand over Username")
	}
	if strings.Contains(wrong, "verso-form__note") {
		t.Error("the failed sign-ins still stand beside the act")
	}
	if strings.Contains(wrong, notice) {
		t.Error("wrong credentials open the form with a notice")
	}

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(bad.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site status = %d, want 403", rec.Code)
	}
	refusedRequest(t, rec.Body.String(), notice, "That request didn’t come from this page — reload and try again.")

	for i := 0; i < loginMaxFailures; i++ {
		postForm(t, srv, "/login", bad)
	}
	limited := postForm(t, srv, "/login", bad)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d, want 429", limited.Code)
	}
	refusedRequest(t, limited.Body.String(), notice, "Too many attempts — wait a minute and try again.")
}

func refusedRequest(t *testing.T, body, notice, words string) {
	t.Helper()
	_, form, found := strings.Cut(body, `<form method="post" action="/login"`)
	if !found {
		t.Fatal("no sign-in form")
	}
	if !strings.Contains(form, notice) || !strings.Contains(form, words) {
		t.Errorf("the form does not open on %q", words)
	}
	if strings.Contains(body, `aria-invalid="true"`) {
		t.Errorf("a refused request marks a field for %q", words)
	}
}

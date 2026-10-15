// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/theme"
)

var (
	testMocha = theme.Theme{ID: "catppuccin-mocha", Name: "Mocha", Family: "Catppuccin", Scheme: theme.Dark,
		Colors: map[string]string{"ground": "#1e1e2e", "canvas": "#11111b"}}
	testSandDark = theme.Theme{ID: theme.SandDark, Name: "Sand", Family: "Sand", Scheme: theme.Dark,
		Colors: map[string]string{"ground": "#1c1a17", "canvas": "#121110"}}
	mochaBlock = `<style id="verso-theme">:root{color-scheme:dark;--color-canvas:#11111b;--color-ground:#1e1e2e;}</style>`
)

// themed is a server drawing in Mocha.
func themed(t *testing.T) *Server {
	t.Helper()
	s := newServer(t, fakeBackend{})
	s.useThemes(theme.Set{testMocha.ID: testMocha, testSandDark.ID: testSandDark}, testMocha.ID)
	return s
}

// carriesTheTheme fails unless body carries the theme block after the
// stylesheet, where it outranks the palette.
func carriesTheTheme(t *testing.T, page, body string) {
	t.Helper()
	at := strings.Index(body, mochaBlock)
	if at < 0 {
		t.Errorf("%s does not carry the theme block %s", page, mochaBlock)
		return
	}
	if css := strings.Index(body, "<style"); css < 0 || css >= at {
		t.Errorf("%s carries the theme block before the stylesheet", page)
	}
}

func TestEveryPageCarriesTheThemeAfterTheStylesheet(t *testing.T) {
	s := themed(t)
	carriesTheTheme(t, "a page", get(t, s, "/devices").Body.String())

	login := httptest.NewRecorder()
	s.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	carriesTheTheme(t, "the sign-in page", login.Body.String())

	started := httptest.NewRecorder()
	s.renderFirmwareStarted(started, httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware", nil))
	carriesTheTheme(t, "the firmware-started takeover", started.Body.String())

	upgrading := httptest.NewRecorder()
	s.renderUpgrading(upgrading, httptest.NewRequest(http.MethodGet, "/", nil))
	carriesTheTheme(t, "the upgrading takeover", upgrading.Body.String())

	restarting, err := s.restartingPage(httptest.NewRequest(http.MethodPost, "/", nil), restartPlan{Title: "Restarting"})
	if err != nil {
		t.Fatal(err)
	}
	carriesTheTheme(t, "the restarting takeover", restarting)
}

// Under Sand light the block is there and empty, so a pick can swap it.
func TestSandLightCarriesAnEmptyThemeBlock(t *testing.T) {
	s := newServer(t, fakeBackend{})
	s.useThemes(theme.Set{testSandDark.ID: testSandDark}, theme.SandLight)
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, `<style id="verso-theme"></style>`) {
		t.Error("the page under Sand light carries no empty theme block")
	}
}

// Under Sand following the browser, a takeover declares one bar colour for
// each preference.
func TestATakeoverDeclaresABarColourPerPreference(t *testing.T) {
	s := newServer(t, fakeBackend{})
	s.useThemes(theme.Set{testSandDark.ID: testSandDark}, theme.Sand)
	restarting, err := s.restartingPage(httptest.NewRequest(http.MethodPost, "/", nil), restartPlan{Title: "Restarting"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<meta name="theme-color" media="(prefers-color-scheme: light)" content="#faf9f7">`,
		`<meta name="theme-color" media="(prefers-color-scheme: dark)" content="#1c1a17">`,
	} {
		if !strings.Contains(restarting, want) {
			t.Errorf("the restarting takeover does not declare %s", want)
		}
	}
}

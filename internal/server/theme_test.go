// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if css := strings.Index(body, `<link rel="stylesheet"`); css < 0 || css >= at {
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

// themeWriter is a backend whose theme writes land in wrote, or fail with err.
func themeWriter(wrote *[]string, err error) fakeBackend {
	return fakeBackend{setTheme: func(sid, id string) error {
		*wrote = append(*wrote, sid+" "+id)
		return err
	}}
}

func TestPickingAThemeWritesItAndAnswersItsStyle(t *testing.T) {
	var wrote []string
	s := newServer(t, themeWriter(&wrote, nil))
	s.useThemes(theme.Set{testMocha.ID: testMocha, testSandDark.ID: testSandDark}, theme.Sand)
	rec := postPlugin(t, s, "/system/theme", url.Values{"theme": {testMocha.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if len(wrote) != 1 || wrote[0] != "test-sid catppuccin-mocha" {
		t.Errorf("wrote %v; want the theme written with the operator's session", wrote)
	}
	if !strings.Contains(rec.Body.String(), mochaBlock) {
		t.Errorf("the answer does not carry the new style %s: %s", mochaBlock, rec.Body.String())
	}
	if got := s.themeNow().choice; got != testMocha.ID {
		t.Errorf("the theme in force is %q; want %q", got, testMocha.ID)
	}
	// The next page draws in it too.
	carriesTheTheme(t, "the next page", get(t, s, "/devices").Body.String())
}

func TestPickingABuiltInSandIsWritten(t *testing.T) {
	for _, id := range []string{theme.Sand, theme.SandLight, theme.SandDark} {
		var wrote []string
		s := newServer(t, themeWriter(&wrote, nil))
		s.useThemes(theme.Set{testSandDark.ID: testSandDark}, testMocha.ID)
		if rec := postPlugin(t, s, "/system/theme", url.Values{"theme": {id}}); rec.Code != http.StatusOK || len(wrote) != 1 {
			t.Errorf("%s: status %d, wrote %v", id, rec.Code, wrote)
		}
	}
}

func TestAThemeNotInstalledIsRefused(t *testing.T) {
	var wrote []string
	s := newServer(t, themeWriter(&wrote, nil))
	s.useThemes(theme.Set{testSandDark.ID: testSandDark}, theme.Sand)
	rec := postPlugin(t, s, "/system/theme", url.Values{"theme": {"gone"}})
	if rec.Code == http.StatusOK || len(wrote) != 0 {
		t.Fatalf("an uninstalled theme: status %d, wrote %v", rec.Code, wrote)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<template data-verso-arrival>") || !strings.Contains(body, "That theme isn&#39;t installed.") {
		t.Errorf("the refusal carries no outcome: %s", body)
	}
	if s.themeNow().choice != theme.Sand {
		t.Error("a refused theme changed the theme in force")
	}
}

func TestAThemeTheRouterRefusesKeepsTheThemeInForce(t *testing.T) {
	var wrote []string
	s := newServer(t, themeWriter(&wrote, errors.New("uci commit failed")))
	s.useThemes(theme.Set{testMocha.ID: testMocha, testSandDark.ID: testSandDark}, theme.Sand)
	rec := postPlugin(t, s, "/system/theme", url.Values{"theme": {testMocha.ID}})
	if rec.Code == http.StatusOK {
		t.Fatalf("a failed write answers %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "The theme wasn&#39;t changed.") {
		t.Errorf("the failure carries no outcome: %s", body)
	}
	if s.themeNow().choice != theme.Sand {
		t.Error("a theme the router refused is in force")
	}
}

func TestPickingAThemeNeedsTheSessionsToken(t *testing.T) {
	var wrote []string
	s := newServer(t, themeWriter(&wrote, nil))
	s.useThemes(theme.Set{testMocha.ID: testMocha}, theme.Sand)
	rec := postForm(t, s, "/system/theme", url.Values{"theme": {testMocha.ID}})
	if rec.Code == http.StatusOK || len(wrote) != 0 {
		t.Errorf("a pick without a session or token: status %d, wrote %v", rec.Code, wrote)
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

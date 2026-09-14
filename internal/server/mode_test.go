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

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// flip posts the sidebar switch the way the browser does: an authenticated,
// CSRF-carrying form asking for one reading and naming the page to return to.
func flip(t *testing.T, srv *Server, mode, next string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess, _ := srv.sessions.get(token)
	form := url.Values{"_csrf": {sess.csrf}, modeField: {mode}, modeNextField: {next}}
	req := httptest.NewRequest(http.MethodPost, modePath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// modeCookieOf returns the reader-mode cookie a response sets, or nil.
func modeCookieOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == modeCookie {
			return c
		}
	}
	return nil
}

// TestModeSwitchRoundTrips: the flip sets the preference, keeps it for a long
// while, and lands the reader back on the page they were reading. Flipping back
// removes the cookie rather than storing a second value — basic is the absence of
// the preference.
func TestModeSwitchRoundTrips(t *testing.T) {
	s := newServer(t, fakeBackend{})

	on := flip(t, s, widget.ModeAdvanced, "/system/access")
	if on.Code != http.StatusSeeOther || on.Header().Get("Location") != "/system/access" {
		t.Fatalf("flip on = %d → %q, want 303 back to the page", on.Code, on.Header().Get("Location"))
	}
	cookie := modeCookieOf(on)
	if cookie == nil || cookie.Value != widget.ModeAdvanced || cookie.MaxAge <= 0 || !cookie.HttpOnly {
		t.Fatalf("mode cookie = %+v, want a durable advanced preference the browser cannot script", cookie)
	}

	off := flip(t, s, widget.ModeBasic, "/")
	if cleared := modeCookieOf(off); cleared == nil || cleared.Value != "" || cleared.MaxAge != -1 {
		t.Fatalf("mode cookie = %+v, want the preference removed", cleared)
	}
}

// TestModeSwitchReturnsSomewhereOnThisShell: the return address comes from a
// form, so a crafted one must not aim the browser off the router.
func TestModeSwitchReturnsSomewhereOnThisShell(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for _, next := range []string{"https://evil.example.com/", "//evil.example.com/", `/\evil.example.com/`, ""} {
		if got := flip(t, s, widget.ModeAdvanced, next).Header().Get("Location"); got != "/" {
			t.Errorf("next %q redirected to %q, want the home page", next, got)
		}
	}
}

// TestModeSwitchIsCSRFGated: flipping is a state change like any other shell
// form, so a signed-in browser without the session's token is refused (VS-04).
func TestModeSwitchIsCSRFGated(t *testing.T) {
	s := newServer(t, fakeBackend{})
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	form := url.Values{modeField: {widget.ModeAdvanced}}
	req := httptest.NewRequest(http.MethodPost, modePath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("a token-less flip = %d, want 403", rec.Code)
	}
	if modeCookieOf(rec) != nil {
		t.Error("a refused flip still set the preference")
	}
}

// TestChromeCarriesNoModeSwitch: no reading has a switch in the rail. The mode
// still exists server-side and still filters page content, but the control that
// flipped it is gone from the chrome until a design says what it looks like.
func TestChromeCarriesNoModeSwitch(t *testing.T) {
	s := newServer(t, fakeBackend{})

	for _, body := range []string{
		get(t, s, "/").Body.String(),
		getMode(t, s, "/", widget.ModeAdvanced).Body.String(),
	} {
		for _, unwanted := range []string{`action="/mode"`, `name="mode" value=`, `role="switch"`} {
			if strings.Contains(body, unwanted) {
				t.Errorf("the chrome still ships the mode switch: %q", unwanted)
			}
		}
	}
}

// TestSidebarListsEveryDestinationInEveryReading: with no switch to reveal them,
// a rail that filtered would be a rail with destinations no one could reach.
// Each entry is an ordinary row — one anatomy, with an icon and no section title
// or rule between them.
func TestSidebarListsEveryDestinationInEveryReading(t *testing.T) {
	m := demoManifest()
	m.Nav = []plugin.NavEntry{{Section: "Network", Label: "DNS", Path: "/dns"}}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})

	for name, body := range map[string]string{
		"basic":    get(t, s, "/").Body.String(),
		"advanced": getMode(t, s, "/", widget.ModeAdvanced).Body.String(),
	} {
		for _, want := range []string{
			`href="/plugins/demo/dns"`,
			`class="flex items-center gap-3 border-l-2 py-2 pr-6 pl-6`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s sidebar missing %q", name, want)
			}
		}
		nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
		for _, unwanted := range []string{"uppercase", "border-t ", "border-t-"} {
			if strings.Contains(nav, unwanted) {
				t.Errorf("%s rail groups its rows with %q; the design has one flat list", name, unwanted)
			}
		}
	}
}

// TestManifestNavModeDoesNotFilterTheSidebar: a plugin may still file an entry
// into one reading (ADR-006), but with no switch in the chrome the rail lists
// every entry — a destination the rail hides is a destination with no way in.
func TestManifestNavModeDoesNotFilterTheSidebar(t *testing.T) {
	m := demoManifest()
	m.Nav = []plugin.NavEntry{
		{Section: "Apps", Label: "Everyday", Path: "/", Mode: widget.ModeBasic},
		{Section: "Apps", Label: "Machinery", Path: "/deep", Mode: widget.ModeAdvanced},
		{Section: "Apps", Label: "Both", Path: "/both"},
	}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})

	for name, body := range map[string]string{
		"basic":    get(t, s, "/").Body.String(),
		"advanced": getMode(t, s, "/", widget.ModeAdvanced).Body.String(),
	} {
		for _, want := range []string{"Everyday", "Machinery", "Both"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s sidebar does not list %q", name, want)
			}
		}
	}
}

// Tier 1 draws the shell's destination icons and honors a manifest's choice.
func TestSidebarDrawsDestinationIcons(t *testing.T) {
	m := demoManifest()
	m.Nav = []plugin.NavEntry{{Section: "Apps", Label: "Demo", Path: "/", Icon: "wifi"}}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})

	body := get(t, s, "/").Body.String()
	nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
	for _, glyph := range []string{
		string(widget.Icon("house", "size-4 shrink-0 text-ink")),
		string(widget.Icon("phone", "size-4 shrink-0 text-glyph")),
		string(widget.Icon("wifi", "size-4 shrink-0 text-glyph")),
		string(widget.Icon("settings", "size-4 shrink-0 text-glyph")),
	} {
		if !strings.Contains(nav, glyph) {
			t.Errorf("sidebar missing destination icon %s", glyph)
		}
	}
}

// TestSystemServicesIsTheAdvancedReading: the tab is absent in basic mode, and the
// page still answers its own URL — mode is a reading aid, never authority
// (ADR-015 §6).
func TestSystemServicesIsTheAdvancedReading(t *testing.T) {
	s := newServer(t, fakeBackend{rcStates: map[string]openwrt.RCState{}})

	basic := get(t, s, "/system/access").Body.String()
	if strings.Contains(basic, `href="/system/services"`) {
		t.Error("the basic System bar offers Services, which belongs to the advanced reading")
	}
	if advanced := getMode(t, s, "/system/access", widget.ModeAdvanced).Body.String(); !strings.Contains(advanced, `href="/system/services"`) {
		t.Error("the advanced System bar is missing Services")
	}
	if rec := get(t, s, "/system/services"); rec.Code != http.StatusOK {
		t.Errorf("GET /system/services in basic mode = %d, want the page to render anyway", rec.Code)
	}
}

// TestEnvelopePagesModeFiltersTheSubpageBar: a plugin's subpage may belong to one
// reading, filtered exactly as a manifest entry is.
func TestEnvelopePagesModeFiltersTheSubpageBar(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Status: http.StatusOK,
		Pages: []plugin.PageTab{
			{Label: "Leases", Path: ""},
			{Label: "Diagnostics", Path: "diagnostics", Mode: widget.ModeAdvanced},
		},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	if basic := get(t, s, "/plugins/demo/").Body.String(); strings.Contains(basic, "Diagnostics") {
		t.Error("the basic subpage bar offers an advanced-only tab")
	}
	if advanced := getMode(t, s, "/plugins/demo/", widget.ModeAdvanced).Body.String(); !strings.Contains(advanced, "Diagnostics") {
		t.Error("the advanced subpage bar is missing the advanced tab")
	}
	// The tab is gone, the page is not: its URL renders in either reading.
	if rec := get(t, s, "/plugins/demo/diagnostics"); rec.Code != http.StatusOK {
		t.Errorf("GET an advanced subpage in basic mode = %d, want it to render", rec.Code)
	}
}

// TestPluginTreeIsFilteredByMode: the shell filters a plugin's own widget tree at
// render, so a basic reader is never sent the advanced markup.
func TestPluginTreeIsFilteredByMode(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"stack","children":[
			{"type":"section","title":"Essentials","children":[
				{"type":"field","name":"name","label":"Name"},
				{"type":"field","name":"family","label":"Address family","advanced":true}
			]},
			{"type":"section","title":"Timers","mode":"advanced","children":[
				{"type":"field","name":"timeout","label":"Timeout"}
			]}
		]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	basic := get(t, s, "/plugins/demo/").Body.String()
	if !strings.Contains(basic, "Essentials") || !strings.Contains(basic, `name="name"`) {
		t.Error("the basic reading lost content that belongs to both readings")
	}
	for _, unwanted := range []string{"Timers", `name="timeout"`, `name="family"`} {
		if strings.Contains(basic, unwanted) {
			t.Errorf("the basic reading ships %q", unwanted)
		}
	}
	advanced := getMode(t, s, "/plugins/demo/", widget.ModeAdvanced).Body.String()
	for _, want := range []string{"Essentials", "Timers", `name="timeout"`, `name="family"`} {
		if !strings.Contains(advanced, want) {
			t.Errorf("the advanced reading is missing %q", want)
		}
	}
}

// TestHomeConnectionFactsAreTheAdvancedReading: the tile strip is the whole
// verdict a basic reader needs; the addresses behind it are the advanced face of
// the same fact (ADR-015 §2).
func TestHomeConnectionFactsAreTheAdvancedReading(t *testing.T) {
	s := newServer(t, fakeBackend{wanConn: openwrt.WANConn{
		V4Proto: "DHCP", V4Addr: "10.0.0.2/24", V6Proto: "DHCPv6", V6Prefix: "fd00::/56",
	}})

	basic := get(t, s, "/").Body.String()
	if !strings.Contains(basic, "Internet") || !strings.Contains(basic, "Firewall") {
		t.Error("the basic home page lost the tile strip")
	}
	for _, unwanted := range []string{"IPv4", "10.0.0.2/24", "fd00::/56"} {
		if strings.Contains(basic, unwanted) {
			t.Errorf("the basic home page ships %q", unwanted)
		}
	}
	advanced := getMode(t, s, "/", widget.ModeAdvanced).Body.String()
	for _, want := range []string{"Internet", "IPv4", "10.0.0.2/24", "IPv6", "fd00::/56"} {
		if !strings.Contains(advanced, want) {
			t.Errorf("the advanced home page is missing %q", want)
		}
	}
}

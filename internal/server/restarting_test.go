// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// section cuts one block of the maintenance page out of the body, from its
// opening id to the next section's, so an assertion about the reboot act cannot
// be satisfied by a confirm that belongs to another section.
func section(t *testing.T, body, from, to string) string {
	t.Helper()
	start := strings.Index(body, from)
	if start < 0 {
		t.Fatalf("page has no %q:\n%s", from, body)
	}
	end := strings.Index(body[start:], to)
	if end < 0 {
		t.Fatalf("page has no %q after %q", to, from)
	}
	return body[start : start+end]
}

// assertRestartingTakeover checks the shape every restart ends on: the three
// server-rendered surfaces the client moves between, its own poller, and no
// shell chrome around it.
func assertRestartingTakeover(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range append([]string{
		`data-verso-restarting`,
		`data-verso-restarting-surface="working"`,
		`data-verso-restarting-surface="back"`,
		`data-verso-restarting-surface="stalled"`,
		`data-verso-restarting-status="/system/maintenance/restart/status"`,
		`<script src="/assets/verso-takeover.js" defer></script>`,
	}, wants...) {
		if !strings.Contains(body, want) {
			t.Errorf("the restarting takeover is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `x-data="sidebar"`) || strings.Contains(body, "<aside") {
		t.Errorf("the restarting takeover is chrome-less — no sidebar/nav:\n%s", body)
	}
}

// TestRebootAsksBeforeItActs: with nothing staged, a reboot still drops every
// device on the network, so the one button asks first rather than acting on a
// single press.
func TestRebootAsksBeforeItActs(t *testing.T) {
	s := newServer(t, fakeBackend{})
	reboot := section(t, get(t, s, "/system/maintenance").Body.String(), `id="section-reboot"`, `id="section-factory-reset"`)
	for _, want := range []string{`x-data="confirm"`, "Reboot now", "Reboot the router now?"} {
		if !strings.Contains(reboot, want) {
			t.Errorf("the reboot section is missing %q:\n%s", want, reboot)
		}
	}
}

// TestFactoryResetAsksForTheHostnameOnly: one deliberate key, typed from what
// the page shows. The signed-in administrator is not asked for a password again.
func TestFactoryResetAsksForTheHostnameOnly(t *testing.T) {
	s := newServer(t, fakeBackend{hn: "verso-lab"})
	body := get(t, s, "/system/maintenance").Body.String()
	reset := body[strings.Index(body, `id="section-factory-reset"`):]
	if !strings.Contains(reset, `name="hostname"`) {
		t.Errorf("the factory reset must ask for the hostname:\n%s", reset)
	}
	if strings.Contains(reset, `name="password"`) {
		t.Errorf("the factory reset must not ask the signed-in administrator for a password:\n%s", reset)
	}
	// One field, its box showing the hostname to type, and the act that
	// destroys under it in the danger tone: a form like any other, which the
	// page's script holds shut until the box says what it shows.
	form := section(t, reset, `action="/system/maintenance/factory-reset"`, "</form>")
	field, button := strings.Index(form, `name="hostname"`), strings.Index(form, "Erase and start over")
	if field < 0 || button < field || !strings.Contains(form, `placeholder="verso-lab"`) || !strings.Contains(form, "bg-crimson") {
		t.Errorf("the hostname field should show the hostname, and the danger act stand under it:\n%s", form)
	}
}

// TestRebootLandsOnTheRestartingTakeover: the reboot answers with a surface that
// watches the router go and come back, not a bare paragraph.
func TestRebootLandsOnTheRestartingTakeover(t *testing.T) {
	rebooted := false
	s := newServer(t, fakeBackend{restart: func(context.Context, string) error { rebooted = true; return nil }})
	res := postPlugin(t, s, "/system/maintenance/restart", url.Values{})
	if res.Code != http.StatusOK || !rebooted {
		t.Fatalf("reboot = %d, rebooted=%v; want 200 and the backend called", res.Code, rebooted)
	}
	assertRestartingTakeover(t, res.Body.String(),
		"Restarting",
		"Closing everything down and coming back on the same firmware. Settings are untouched.",
		`data-verso-restarting-budget="180"`,
	)
}

// TestFactoryResetChecksTheHostnameOnTheServer: the typed hostname is the
// reset's one key, so the server holds it too; a form posted without the page's
// script must not erase the router. The session is already the administrator,
// so no password is asked: a verifier that refuses everything changes nothing.
func TestFactoryResetChecksTheHostnameOnTheServer(t *testing.T) {
	reset := false
	s := newServerFull(t, fakeBackend{hn: "verso-lab", factoryReset: func(context.Context, string) error { reset = true; return nil }},
		&fakeTransport{}, nil, fakeAuth{sid: "test-sid", verifyErr: errors.New("no password is asked")})

	for _, typed := range []string{"", "verso", "VERSO-LAB"} {
		res := postPlugin(t, s, "/system/maintenance/factory-reset", url.Values{"hostname": {typed}})
		if reset {
			t.Fatalf("hostname %q erased the router", typed)
		}
		if res.Code != http.StatusSeeOther || res.Header().Get("Location") != maintenancePath {
			t.Errorf("hostname %q = %d → %q; want a redirect back to Maintenance", typed, res.Code, res.Header().Get("Location"))
		}
	}

	res := postPlugin(t, s, "/system/maintenance/factory-reset", url.Values{"hostname": {"verso-lab"}})
	if res.Code != http.StatusOK || !reset {
		t.Fatalf("factory reset = %d, reset=%v; want 200 and the backend called", res.Code, reset)
	}
	assertRestartingTakeover(t, res.Body.String(), "Erasing and restarting", `data-verso-restarting-budget="300"`)
}

// TestRestartStatusAnswersWhileTheSessionLives: the takeover's poll. While the
// router is still up the answer is JSON; after the restart the session is gone
// and the same request meets the login redirect instead, which is the client's
// "back" signal.
func TestRestartStatusAnswersWhileTheSessionLives(t *testing.T) {
	s := newServer(t, fakeBackend{})
	res := get(t, s, "/system/maintenance/restart/status")
	if res.Code != http.StatusOK || !strings.HasPrefix(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status = %d %q; want 200 JSON", res.Code, res.Header().Get("Content-Type"))
	}
}

// TestFactoryLANAddress reads where the router answers after a reset the way
// config_generate decides it: a static LAN takes the board's own ipaddr, or
// 192.168.1.1; anything else has no fixed address to name.
func TestFactoryLANAddress(t *testing.T) {
	for _, tc := range []struct{ name, board, want string }{
		{"static without an address", `{"network":{"lan":{"device":"eth0","protocol":"static"}}}`, "192.168.1.1"},
		{"static with the board's address", `{"network":{"lan":{"ports":["lan1","lan2"],"protocol":"static","ipaddr":"10.0.0.1"}}}`, "10.0.0.1"},
		{"dhcp", `{"network":{"lan":{"device":"eth0","protocol":"dhcp"}}}`, ""},
		{"no lan device", `{"network":{"lan":{"protocol":"static"}}}`, ""},
		{"no lan", `{"network":{"wan":{"device":"eth1","protocol":"dhcp"}}}`, ""},
		{"unreadable", `{`, ""},
	} {
		if got := factoryLANAddress([]byte(tc.board)); got != tc.want {
			t.Errorf("%s: factoryLANAddress = %q, want %q", tc.name, got, tc.want)
		}
	}
}

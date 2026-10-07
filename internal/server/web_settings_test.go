// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/listen"
)

// webSettings is Access on a router whose verso config holds web, with the
// writes it stages recorded.
type webSettings struct {
	s       *Server
	writes  []uciWrite
	deletes []string
	adds    []string
}

func newWebSettings(t *testing.T, ports string, web map[string]any) *webSettings {
	t.Helper()
	w := &webSettings{}
	verso := map[string]any{}
	if web != nil {
		verso["web"] = web
	}
	w.s = newServer(t, fakeBackend{
		uci:     map[string]map[string]any{"verso": verso},
		writes:  &w.writes,
		deletes: &w.deletes,
		adds:    &w.adds,
	})
	w.s.SetListeners(listenersHolding(ports))
	return w
}

func (w *webSettings) post(t *testing.T, field, value string) int {
	t.Helper()
	return postPlugin(t, w.s, "/system/access", url.Values{field: {value}}).Code
}

func TestAccessShowsWhereTheWebInterfaceAnswers(t *testing.T) {
	body := get(t, newWebSettings(t, listen.PortsOwn, nil).s, "/system/access").Body.String()
	for _, want := range []string{"HTTPS port", `name="web_https" value="443"`, "HTTP port", `name="web_http" value="80"`, "Redirect HTTP to HTTPS", "web.redirect_https"} {
		if !strings.Contains(body, want) {
			t.Errorf("the web interface's settings are missing %q", want)
		}
	}
	set := newWebSettings(t, listen.PortsSet, map[string]any{"listen_https": []any{"0.0.0.0:8443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"})
	body = get(t, set.s, "/system/access").Body.String()
	if !strings.Contains(body, `name="web_https" value="8443"`) || !strings.Contains(body, `name="web_http" value="8080"`) {
		t.Error("the settings do not read the ports verso.web sets")
	}
	// Beside LuCI the ports are where LuCI leaves them; its section says so.
	if body := get(t, newWebSettings(t, listen.PortsBeside, nil).s, "/system/access").Body.String(); strings.Contains(body, `name="web_https"`) {
		t.Error("beside LuCI, the ports are offered for editing")
	}
}

func TestAPortStagesOnEveryAddressItWasBoundTo(t *testing.T) {
	w := newWebSettings(t, listen.PortsOwn, nil)
	if code := w.post(t, "web_https", "9443"); code >= http.StatusBadRequest {
		t.Fatalf("status %d", code)
	}
	if len(w.adds) != 1 || len(w.writes) != 1 || fmt.Sprint(w.writes[0].values["listen_https"]) != "[0.0.0.0:9443 [::]:9443]" {
		t.Fatalf("adds %v, writes %+v", w.adds, w.writes)
	}

	w = newWebSettings(t, listen.PortsSet, map[string]any{"listen_https": []any{"192.168.1.1:8443"}})
	w.post(t, "web_https", "9443")
	if len(w.writes) != 1 || fmt.Sprint(w.writes[0].values["listen_https"]) != "[192.168.1.1:9443]" {
		t.Errorf("an address bound by hand was not kept: %+v", w.writes)
	}
}

func TestThePortTheRouterAnswersOnAnywayLeavesNothingWritten(t *testing.T) {
	w := newWebSettings(t, listen.PortsSet, map[string]any{"listen_https": []any{"0.0.0.0:9443", "[::]:9443"}})
	w.post(t, "web_https", "443")
	if len(w.writes) != 0 || len(w.deletes) != 1 || w.deletes[0] != "verso.web.listen_https" {
		t.Errorf("writes %+v, deletes %v", w.writes, w.deletes)
	}
}

func TestAPortThatCannotBeIsRefused(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"web_https", "abc"}, {"web_https", "0"}, {"web_http", "70000"}, {"web_http", "443"},
	} {
		w := newWebSettings(t, listen.PortsOwn, nil)
		if code := w.post(t, tc.field, tc.value); code != http.StatusUnprocessableEntity || len(w.writes)+len(w.deletes) != 0 {
			t.Errorf("%s=%s: %d, writes %+v deletes %v", tc.field, tc.value, code, w.writes, w.deletes)
		}
	}
}

func TestTheRedirectStagesOffAsZeroAndOnAsNothing(t *testing.T) {
	w := newWebSettings(t, listen.PortsOwn, nil)
	w.post(t, "web_redirect", "off")
	if len(w.writes) != 1 || w.writes[0].values["redirect_https"] != "0" {
		t.Errorf("off: %+v", w.writes)
	}
	w = newWebSettings(t, listen.PortsSet, map[string]any{"redirect_https": "0"})
	w.post(t, "web_redirect", "on")
	if len(w.writes) != 0 || len(w.deletes) != 1 || w.deletes[0] != "verso.web.redirect_https" {
		t.Errorf("on: writes %+v, deletes %v", w.writes, w.deletes)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/i18n"
)

func TestLoginStatusExposesOnlyPublicFacts(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	// Neither a public render nor its refresh may borrow privileged dependencies.
	srv.backend, srv.transport = nil, nil
	for _, tc := range []struct {
		name string
		up   bool
		err  error
		want string
	}{
		{"up", true, nil, "true"},
		{"down", false, nil, "false"},
		{"unreadable", false, errors.New("unavailable"), "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv.loginInternet = func() (bool, error) { return tc.up, tc.err }
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login/status", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			var facts map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &facts); err != nil {
				t.Fatal(err)
			}
			allowed := map[string]bool{"internet": true, "uptime": true, "clock": true, "zone": true, "unix": true, "offset": true}
			for key := range facts {
				if !allowed[key] {
					t.Errorf("public status exposes %q", key)
				}
			}
			if string(facts["internet"]) != tc.want {
				t.Errorf("internet = %s, want %s", facts["internet"], tc.want)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Error("public status created a session")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("live status may not be cached")
			}
			rec = httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("public page = %d", rec.Code)
			}
		})
	}
}

func TestLoginNetworkOnlyNamesTheVisitorsSubnet(t *testing.T) {
	var addresses []net.Addr
	for _, cidr := range []string{"10.0.0.1/16", "10.0.20.1/24", "192.0.2.1/24", "fd00:1234::1/64"} {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, &net.IPNet{IP: ip, Mask: network.Mask})
	}
	for _, tc := range []struct{ ip, want string }{
		{"10.0.20.42", "10.0.20.0/24"},
		{"::ffff:10.0.20.42", "10.0.20.0/24"},
		{"fd00:1234::42", "fd00:1234::/64"},
		{"203.0.113.9", ""},
		{"127.0.0.1", ""},
		{"<script>", ""},
	} {
		if got := matchingNetwork(tc.ip, addresses); got != tc.want {
			t.Errorf("network(%q) = %q, want %q", tc.ip, got, tc.want)
		}
	}
}

func TestLoginSlovenianCoversRestingErrorsAndLiveState(t *testing.T) {
	bundle, problems := i18n.Load(os.DirFS("../../i18n"), "*/*.json")
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	missing := map[string]bool{}
	bundle = bundle.Recorded(func(code, component, key string, translated bool) {
		if code == "sl" && !translated {
			missing[key] = true
		}
	})
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{err: errors.New("denied")})
	srv.SetBundle(bundle)
	srv.loginInternet = func() (bool, error) { return true, nil }
	for _, path := range []string{"/login", "/login?expired=1", "/login/status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept-Language", "sl-SI,sl;q=0.9")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		if path == "/login" {
			for _, text := range []string{"Prijava", "Prijavi se", "Prijavljanje…", "Povezava v internet deluje", "Lokalni čas", "Uporabniško ime"} {
				if !strings.Contains(rec.Body.String(), text) {
					t.Errorf("login missing %q", text)
				}
			}
		}
	}
	for attempt := 1; attempt <= loginMaxFailures+1; attempt++ {
		form := url.Values{"username": {`operator<&>`}, "password": {"do-not-render-this"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept-Language", "sl")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.Contains(body, "do-not-render-this") {
			t.Error("password echoed on failure")
		}
		if attempt <= loginMaxFailures && !strings.Contains(body, `value="operator&lt;&amp;&gt;"`) {
			t.Error("submitted username was not preserved and escaped")
		}
		if attempt == 2 && !strings.Contains(body, "2 neuspela poskusa") {
			t.Error("dual count was not translated")
		}
		if attempt > loginMaxFailures && rec.Code != http.StatusTooManyRequests {
			t.Errorf("lockout status = %d", rec.Code)
		}
	}
	for _, secs := range []int64{0, 60, 3600, 86400} {
		loginDuration(bundle.Translator("sl"), secs)
	}
	for key := range missing {
		t.Errorf("sign-in falls back to English for %q", key)
	}
}

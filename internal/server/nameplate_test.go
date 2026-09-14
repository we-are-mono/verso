// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type nameplateBackend struct {
	fakeBackend
	readHostname func() (string, error)
}

func (b nameplateBackend) Hostname(context.Context, string) (string, error) {
	return b.readHostname()
}

func TestNameplateFollowsAppliedRenamesAndRollbacks(t *testing.T) {
	name := "router-before"
	var readErr error
	s := newServer(t, nameplateBackend{readHostname: func() (string, error) { return name, readErr }})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, active := range []string{"router-before", "router-after", "router-before"} {
		name = active
		if got := s.nameplate(r); got != active {
			t.Fatalf("nameplate = %q, want current hostname %q", got, active)
		}
	}
	readErr = errors.New("hostname unavailable")
	if got := s.nameplate(r); got != "" {
		t.Fatalf("unreadable hostname = %q, want localized template fallback", got)
	}
}

func TestHeadersAgreeOnRunningHostname(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t, nameplateBackend{
		fakeBackend:  fakeBackend{hn: "different-configured-name"},
		readHostname: os.Hostname,
	})
	for _, route := range []string{"/login", "/"} {
		var body string
		if route == "/login" {
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
			body = rec.Body.String()
		} else {
			body = get(t, s, route).Body.String()
		}
		start := strings.Index(body, "<header")
		if start < 0 {
			t.Fatalf("%s: no header", route)
		}
		header := strings.SplitN(body[start:], "</header>", 2)[0]
		if !strings.Contains(header, want) || strings.Contains(header, "different-configured-name") {
			t.Errorf("%s header must report running hostname %q", route, want)
		}
	}
}

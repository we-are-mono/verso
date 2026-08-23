// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// fakeBackend is a Backend seam double: no ubus/uci, no device needed.
type fakeBackend struct {
	si  openwrt.SystemInfo
	hn  string
	err error
}

func (f fakeBackend) SystemInfo(context.Context) (openwrt.SystemInfo, error) {
	return f.si, f.err
}

func (f fakeBackend) Hostname(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.hn, nil
}

func newServer(t *testing.T, backend openwrt.Backend) *Server {
	t.Helper()
	r, err := widget.NewRenderer()
	if err != nil {
		t.Fatalf("widget.NewRenderer: %v", err)
	}
	s, err := New(r, backend)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHealthzReturnsOK(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{}), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok\n"; got != want {
		t.Fatalf("GET /healthz: body = %q, want %q", got, want)
	}
}

func TestIndexRendersRealData(t *testing.T) {
	backend := fakeBackend{
		hn: "verso-lab",
		si: openwrt.SystemInfo{
			Uptime: 3661,
			Load:   [3]int64{65536, 0, 0},
			Memory: openwrt.Memory{Total: 64883740672, Available: 37969338368},
		},
	}
	rec := get(t, newServer(t, backend), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"<table", "verso-lab", "1h 1m", "1.00", "GiB"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
}

func TestIndexDegradesWhenBackendFails(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{err: errors.New("ubus down")}), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200 (must degrade, not 500)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("GET /: expected 'unavailable' degradation")
	}
}

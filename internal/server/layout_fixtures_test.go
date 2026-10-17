// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build layoutfixtures

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteLayoutFixtures writes the pages the dev container cannot be asked
// for — the takeovers, which stand only while the router restarts or flashes —
// as the shell renders them, self-contained (each inlines its stylesheet), for
// the layout snapshot to record beside the live pages (make layout-snapshot).
// Each is its first surface; the client moves [hidden] between the rest.
func TestWriteLayoutFixtures(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "build", "layout-fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, fakeBackend{})
	tr := func(s string) string { return s }
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for name, plan := range map[string]restartPlan{
		"restarting":    rebootPlan(tr),
		"restoring":     restorePlan(tr),
		"factory-reset": factoryResetPlan(tr, "192.168.1.1"),
	} {
		page, err := s.restartingPage(httptest.NewRequest(http.MethodPost, "/", nil), plan)
		if err != nil {
			t.Fatal(err)
		}
		write(name, page)
	}

	upgrading := httptest.NewRecorder()
	s.renderUpgrading(upgrading, httptest.NewRequest(http.MethodGet, "/", nil))
	write("upgrading", upgrading.Body.String())

	started := httptest.NewRecorder()
	s.renderFirmwareStarted(started, httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware", nil))
	write("firmware-started", started.Body.String())
}

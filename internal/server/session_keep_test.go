// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestASessionKeptAcrossARestartIsTheSameSession: a store written out and read
// back into a fresh one holds the same sessions — the same cookie token, the
// same CSRF token and rpcd sid, the same idle and absolute clocks — so a
// browser signed in before the restart is still signed in after it, and a form
// it rendered before still posts.
func TestASessionKeptAcrossARestartIsTheSameSession(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	before := newSessionsClock(clk.now)
	token := before.CreateWithMetadata("sid-1", "root", "10.0.0.2", "Firefox")
	clk.advance(10 * time.Minute)
	was, _ := before.get(token)

	path := filepath.Join(t.TempDir(), "sessions.json")
	if err := before.keep(path); err != nil {
		t.Fatalf("keep: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the kept sessions are bearer tokens, readable by the shell alone: %v %v", info.Mode(), err)
	}

	after := newSessionsClock(clk.now)
	if err := after.restore(path); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, ok := after.peek(token)
	if !ok {
		t.Fatal("the session signed in before the restart is gone after it")
	}
	// Times are compared as instants: a written time keeps neither the
	// process's monotonic reading nor its *Location, and needs neither.
	sameTimes := got.created.Equal(was.created) && got.lastSeen.Equal(was.lastSeen)
	got.created, got.lastSeen, was.created, was.lastSeen = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if got != was || !sameTimes {
		t.Errorf("restored session differs:\n got %+v\nwant %+v", got, was)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a kept store is read once: the file goes when it is restored")
	}
}

// TestARestoredStoreKeepsNoSessionPastItsEnd: the clocks keep running while the
// shell is down, so a session that ran out in the meantime is not brought back.
func TestARestoredStoreKeepsNoSessionPastItsEnd(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	before := newSessionsClock(clk.now)
	token := before.CreateWithMetadata("sid-1", "root", "", "")
	path := filepath.Join(t.TempDir(), "sessions.json")
	if err := before.keep(path); err != nil {
		t.Fatal(err)
	}

	clk.advance(sessionIdleTimeout + time.Minute)
	after := newSessionsClock(clk.now)
	if err := after.restore(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := after.peek(token); ok {
		t.Error("a session idle past its window came back")
	}
	if n := len(after.list()); n != 0 {
		t.Errorf("restored %d expired session(s)", n)
	}
}

// TestADevShellHandsItsSessionsToTheNext: under scripts/dev.sh a redeploy
// replaces the shell mid-edit, so the one going away keeps its sessions and the
// one starting takes them — the browser signed in before the redeploy is still
// signed in after it, and its unsaved form can still be saved.
func TestADevShellHandsItsSessionsToTheNext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-sessions.json")
	before := newServer(t, &fakeBackend{})
	before.keptSessions = path
	token := before.sessions.CreateWithMetadata("test-sid", "root", "", "")
	before.Close()

	after := newServer(t, &fakeBackend{})
	after.keptSessions = path
	after.restoreKeptSessions()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	after.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed in before the redeploy, %d after it (%s)", rec.Code, rec.Header().Get("Location"))
	}
}

// TestADeviceKeepsNoSessionsOnDisk: outside a dev session nothing is written
// when the shell stops — a device's sessions end with the process that holds
// them.
func TestADeviceKeepsNoSessionsOnDisk(t *testing.T) {
	if srv := newServer(t, &fakeBackend{}); srv.keptSessions != "" {
		t.Errorf("a shell outside scripts/dev.sh keeps its sessions at %s", srv.keptSessions)
	}
}

// TestRestoringNothingIsNoError: the first start has nothing kept, and that is
// the ordinary case, not a failure.
func TestRestoringNothingIsNoError(t *testing.T) {
	s := newSessions()
	if err := s.restore(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Errorf("restore of an absent file: %v", err)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// firmwareTruth is the available-firmware answer the takeover reads the target
// version from — the build the router is moving to, named once on the success
// surface.
func firmwareTruth() updatecheck.Truth {
	return updatecheck.Truth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{
		State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051",
	}}
}

// decodeStatus reads the takeover's status endpoint the way its client does.
func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) struct {
	State, Message, Version string
} {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("status Content-Type = %q, want application/json (the client keys on it)", ct)
	}
	var out struct{ State, Message, Version string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("status body is not JSON (%v): %s", err, rec.Body.String())
	}
	return out
}

// TestUpgradingRunningPaintsPreparing: while owut works, the takeover is the
// "preparing" state, and it carries every working surface the client swaps
// between — preparing, installing, restarting, done — pre-rendered server-side so
// the swaps need no fetch of markup.
func TestUpgradingRunningPaintsPreparing(t *testing.T) {
	idleUpdates(t)
	blocked := make(chan struct{})
	s := newServer(t, blockingFirmwareBackend{fakeBackend: fakeBackend{access: true}, blocked: blocked})
	knownUpdates(t, s, firmwareTruth())

	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	body := get(t, s, "/system/maintenance").Body.String()

	for _, want := range []string{
		`data-verso-upgrading-state="preparing"`,
		`data-verso-upgrading-surface="preparing"`,
		`data-verso-upgrading-surface="installing"`,
		`data-verso-upgrading-surface="restarting"`,
		`data-verso-upgrading-surface="done"`,
		"Preparing your new firmware",
		"You&#39;re on Mono OpenWrt 25.12.5 r33051", // the done surface, pre-rendered, hidden
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the running takeover is missing %q:\n%s", want, body)
		}
	}
	// Let the blocked run settle before the test's cleanup resets the shared
	// job, so a still-running goroutine cannot leak state into the next test.
	close(blocked)
	waitForCheck(t)
}

// TestUpgradingDonePhaseShowsInstalling: a clean finish (the install has spawned)
// is the "installing" state — the router is about to restart. On real hardware
// the flash takes this process down moments later; a reload in that window must
// still land on the takeover, not flash the ordinary page.
func TestUpgradingDonePhaseShowsInstalling(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true}) // FirmwareUpgrade returns nil -> done
	knownUpdates(t, s, firmwareTruth())

	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	waitForCheck(t)

	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, `data-verso-upgrading-state="installing"`) {
		t.Errorf("a spawned install should paint the installing state:\n%s", body)
	}
	if !strings.Contains(body, "Installing your new firmware") {
		t.Errorf("the installing surface is missing its copy:\n%s", body)
	}
}

// TestUpgradingStatusReportsThePhase: the small truth the client polls tracks the
// job — working while owut runs, done once the install has spawned.
func TestUpgradingStatusReportsThePhase(t *testing.T) {
	idleUpdates(t)
	blocked := make(chan struct{})
	s := newServer(t, blockingFirmwareBackend{fakeBackend: fakeBackend{access: true}, blocked: blocked})
	knownUpdates(t, s, firmwareTruth())

	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	working := decodeStatus(t, get(t, s, "/system/maintenance/updates/firmware/status"))
	if working.State != "working" {
		t.Errorf("mid-run status state = %q, want working", working.State)
	}
	if working.Version != "25.12.5 r33051" {
		t.Errorf("status version = %q, want the target build", working.Version)
	}

	close(blocked)
	waitForCheck(t)
	done := decodeStatus(t, get(t, s, "/system/maintenance/updates/firmware/status"))
	if done.State != "done" {
		t.Errorf("spawned-install status state = %q, want done", done.State)
	}
}

// TestUpgradingStatusReportsFailure: a failed upgrade reports "failed" and the
// tool's own words, so a client that reaches the failure mid-poll can reload onto
// the server-rendered surface.
func TestUpgradingStatusReportsFailure(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, firmwareUpgradeErr: errors.New("Build failed with status 500")})
	knownUpdates(t, s, firmwareTruth()) // an available update is the precondition to install one
	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	waitForCheck(t)

	status := decodeStatus(t, get(t, s, "/system/maintenance/updates/firmware/status"))
	if status.State != "failed" {
		t.Errorf("failed-run status state = %q, want failed", status.State)
	}
	if !strings.Contains(status.Message, "Build failed with status 500") {
		t.Errorf("status message = %q, want the tool's own words", status.Message)
	}
}

// TestUpgradeTargetVersionNamesOnlyRecordedTarget: the version named on the done
// surface comes only from the recorded check, never from the board. The board
// reports the build the router is running, which before the reboot is the old
// one — naming that on "done" would state the wrong version, so without a recorded
// target the surface stays version-less rather than confidently wrong.
func TestUpgradeTargetVersionNamesOnlyRecordedTarget(t *testing.T) {
	idleUpdates(t)
	// A board that reports the running (pre-upgrade) build; the target must never
	// fall back to it.
	s := newServer(t, fakeBackend{access: true, board: openwrt.Board{Firmware: "25.12.4 r32933"}})
	if v := s.upgradeTargetVersion(); v != "" {
		t.Errorf("target with no recorded check = %q, want empty (never the board's running build)", v)
	}
	knownUpdates(t, s, firmwareTruth())
	if v := s.upgradeTargetVersion(); v != "25.12.5 r33051" {
		t.Errorf("target = %q, want the recorded to-build", v)
	}
}

// TestFirmwareInstallRefusedWithoutAvailableUpdate: the firmware act refuses to
// start when the recorded check offers no newer build. A run there would only
// hand owut a build that does not exist and fail on the server, so the page stays
// the ordinary chromed one — no takeover, and the job never leaves idle.
func TestFirmwareInstallRefusedWithoutAvailableUpdate(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	// No knownUpdates: nothing is on offer.
	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	waitForCheck(t)

	if status := decodeStatus(t, get(t, s, "/system/maintenance/updates/firmware/status")); status.State != "idle" {
		t.Errorf("firmware job state = %q, want idle (the install must not have started)", status.State)
	}
	if body := get(t, s, "/system/maintenance").Body.String(); strings.Contains(body, "data-verso-upgrading") {
		t.Errorf("a refused install must not paint the takeover:\n%s", body)
	}
}

// TestUpgradingDismissReleasesARunningTakeover: the countdown's door works from
// the first screen. While a run is still under way (the build, before the flash
// even begins), dismissing releases the takeover — the screen must never be a
// trap — and the ordinary page returns even though the background run continues.
func TestUpgradingDismissReleasesARunningTakeover(t *testing.T) {
	idleUpdates(t)
	blocked := make(chan struct{})
	s := newServer(t, blockingFirmwareBackend{fakeBackend: fakeBackend{access: true}, blocked: blocked})
	knownUpdates(t, s, firmwareTruth())

	postPlugin(t, s, "/system/maintenance/updates/firmware", nil)
	if !strings.Contains(get(t, s, "/system/maintenance").Body.String(), `data-verso-upgrading`) {
		t.Fatal("a running upgrade should paint the takeover before it is released")
	}

	// The person leaves after the countdown ran out, mid-run.
	if rec := postPlugin(t, s, "/system/maintenance/updates/firmware/dismiss", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("dismiss POST = %d, want a redirect", rec.Code)
	}
	if !firmwareUpgrade.running() {
		t.Error("dismiss must not stop the background run — only release the screen")
	}
	body := get(t, s, "/system/maintenance").Body.String()
	if strings.Contains(body, "data-verso-upgrading") {
		t.Errorf("a released takeover must not re-trap a still-running job:\n%s", body)
	}
	if !strings.Contains(body, `x-data="sidebar"`) {
		t.Errorf("the ordinary maintenance page returns after release:\n%s", body)
	}

	close(blocked)
	waitForCheck(t)
}

// TestUpgradingStatusIdleWhenNothingRuns: with no upgrade active the endpoint says
// so — the "the router came back" signal the client reads after a restart.
func TestUpgradingStatusIdleWhenNothingRuns(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	status := decodeStatus(t, get(t, s, "/system/maintenance/updates/firmware/status"))
	if status.State != "idle" {
		t.Errorf("resting status state = %q, want idle", status.State)
	}
}

// TestUpgradingStatusGatedBySession: the status endpoint is behind the same
// session gate as every route — a browser with no session gets the login
// redirect, which is exactly the signal the client reads as "back on the new
// build" once the router has restarted.
func TestUpgradingStatusGatedBySession(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	req := httptest.NewRequest(http.MethodGet, "/system/maintenance/updates/firmware/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated status = %d, want a redirect to login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Errorf("unauthenticated status redirect = %q, want /login", loc)
	}
}

// TestUpgradingIdleRendersOrdinaryPage: with no firmware job the maintenance area
// is the ordinary chromed page — the takeover only exists while a job owns it.
func TestUpgradingIdleRendersOrdinaryPage(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, firmwareTruth())
	body := get(t, s, "/system/maintenance").Body.String()
	if strings.Contains(body, "data-verso-upgrading") {
		t.Errorf("no job runs, so the takeover must not paint:\n%s", body)
	}
	if !strings.Contains(body, `x-data="sidebar"`) {
		t.Errorf("the ordinary maintenance page carries the shell chrome:\n%s", body)
	}
}

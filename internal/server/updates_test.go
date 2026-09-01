// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// idleUpdates puts the device-wide update jobs back to a known empty state. The
// jobs are one router's, not one test's, so every test starts from "nothing
// running, nothing failed".
func idleUpdates(t *testing.T) {
	t.Helper()
	reset := func() {
		for _, job := range []*backgroundJob{&updateChecks, &packageUpgrade, &feedRefresh} {
			job.mu.Lock()
			job.active, job.failure = false, nil
			job.mu.Unlock()
		}
	}
	reset()
	t.Cleanup(reset)
}

// knownUpdates records a completed check's answer where every surface reads it,
// so a render test exercises the surfaces rather than the background job's
// timing — and takes the same path the daily cron run does.
func knownUpdates(t *testing.T, s *Server, truth updatecheck.Truth) {
	t.Helper()
	if err := updatecheck.Write(s.stateDir, truth); err != nil {
		t.Fatalf("recording the update truth: %v", err)
	}
}

// waitForCheck blocks until the background check settles, the way a person's next
// page load does.
func waitForCheck(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for updateChecks.running() || packageUpgrade.running() {
		if time.Now().After(deadline) {
			t.Fatal("the background update check never finished")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestUpdateCheckRecordsTheAnswerOnce: an on-demand check reads both lanes in the
// background, answers the browser at once, writes what it found where every
// surface reads it, and refuses a second concurrent run.
func TestUpdateCheckRecordsTheAnswerOnce(t *testing.T) {
	idleUpdates(t)
	backend := fakeBackend{
		access:        true,
		pkgUpgradable: []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
		firmware:      openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933"},
	}
	s := newServer(t, backend)

	if _, known := s.updateTruth(); known {
		t.Fatal("a fresh router should hold no recorded answer")
	}
	rec := postPlugin(t, s, "/system/maintenance/updates/check", url.Values{})
	if rec.Code != 303 {
		t.Fatalf("check POST = %d, want a redirect back to the page", rec.Code)
	}
	waitForCheck(t)

	truth, known := s.updateTruth()
	if !known || len(truth.Packages) != 1 || truth.Packages[0].Name != "verso" {
		t.Fatalf("recorded answer = %+v known=%v", truth, known)
	}
	if truth.Firmware.State != openwrt.FirmwareCurrent {
		t.Errorf("firmware lane = %q, want the check's answer", truth.Firmware.State)
	}
	if !truth.Pending() {
		t.Error("an upgradable package is something to install")
	}
	if truth.CheckedAt.IsZero() {
		t.Error("an answer with no timestamp cannot be shown as fresh or stale")
	}
}

// TestUpdateCheckSurvivesAShellRestart: the answer is a file on the device, so a
// process that never ran the check still reports what the last one found — which
// is what makes the daily cron run worth having.
func TestUpdateCheckSurvivesAShellRestart(t *testing.T) {
	idleUpdates(t)
	first := newServer(t, fakeBackend{access: true})
	knownUpdates(t, first, updatecheck.Truth{
		CheckedAt: time.Now().Add(-3 * time.Hour),
		Packages:  []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
	})

	restarted := newServer(t, fakeBackend{access: true})
	restarted.stateDir = first.stateDir
	body := get(t, restarted, "/system/maintenance").Body.String()
	for _, want := range []string{"Checked", "3 h ago", "Verso 0.0.23 is available"} {
		if !strings.Contains(body, want) {
			t.Errorf("a restarted shell should read the recorded answer, missing %q:\n%s", want, body)
		}
	}
}

// TestUpdateCheckRefusesASecondRun: the check is one act device-wide.
func TestUpdateCheckRefusesASecondRun(t *testing.T) {
	idleUpdates(t)
	blocked := make(chan struct{})
	started := updateChecks.start(func() error {
		<-blocked
		return nil
	})
	if !started {
		t.Fatal("an idle job should accept the first run")
	}
	if updateChecks.start(func() error { return nil }) {
		t.Error("a second check should be refused while one runs")
	}
	close(blocked)
	waitForCheck(t)
}

// TestUpdateCheckKeepsTheLastAnswerOnFailure: a failed run states itself once and
// leaves the previous answer standing — a stale truth beats no truth.
func TestUpdateCheckKeepsTheLastAnswerOnFailure(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, pkgUpgradableErr: errors.New("helper unreachable")})
	knownUpdates(t, s, updatecheck.Truth{
		Packages:  []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}},
		CheckedAt: time.Now().Add(-2 * time.Hour),
	})

	postPlugin(t, s, "/system/maintenance/updates/check", url.Values{})
	waitForCheck(t)

	truth, known := s.updateTruth()
	if !known || len(truth.Packages) != 1 {
		t.Fatalf("a failed check should not erase the previous answer: %+v", truth)
	}
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "Update check failed") {
		t.Errorf("the failure should be stated once on the next render:\n%s", body)
	}
	if strings.Contains(get(t, s, "/system/maintenance").Body.String(), "Update check failed") {
		t.Error("a failure already seen should not be repeated")
	}
}

// TestMaintenanceUpdatesNeverChecked: a router that has not looked says exactly
// that, and offers the check — no invented "up to date", no install offer.
func TestMaintenanceUpdatesNeverChecked(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		"Updates", "has not looked for updates yet",
		"Check for updates", `action="/system/maintenance/updates/check"`,
		"is not known until this router checks",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the never-checked Updates section is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Update now") {
		t.Error("a router that has not checked has nothing to offer installing")
	}
}

// TestMaintenanceUpdatesSoftwareLane: Verso's own package leads by name, the rest
// fold behind the seam, and one act installs them all.
func TestMaintenanceUpdatesSoftwareLane(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, updatecheck.Truth{
		CheckedAt: time.Now(),
		Packages: []openwrt.PackageUpgrade{
			{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
			{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
			{Name: "rpcd", Installed: "2025.12.03", Available: "2026.07.19"},
			{Name: "ubus", Installed: "2025.12.02", Available: "2026.06.28"},
			{Name: "uhttpd", Installed: "2025.10.03", Available: "2026.06.16"},
		},
	})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		"Verso 0.0.23 is available", "this router runs 0.0.22",
		"Updating also brings 4 other packages", "Checked", "just now",
		"2.91-r3 → 2.93-r1",         // the manifest states each transition verbatim
		"What changes — 5 packages", // the full manifest folds behind a disclosure
		"Update now", `action="/system/maintenance/updates/install"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the software lane is missing %q:\n%s", want, body)
		}
	}
}

// TestMaintenanceUpdatesVersoAlone: Verso as the only upgradable package says so
// and claims no company it does not have.
func TestMaintenanceUpdatesVersoAlone(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, updatecheck.Truth{
		CheckedAt: time.Now(),
		Packages:  []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
	})
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "Verso 0.0.23 is available") {
		t.Errorf("Verso's own update should lead by name:\n%s", body)
	}
	if strings.Contains(body, "Updating also brings") || strings.Contains(body, "What changes") {
		t.Error("a lead that is the whole story needs no manifest behind it")
	}
}

// TestMaintenanceUpdatesSoftwareLaneWithoutVerso: with Verso itself current, the
// set speaks as a count and the manifest table carries the detail.
func TestMaintenanceUpdatesSoftwareLaneWithoutVerso(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, updatecheck.Truth{
		CheckedAt: time.Now(),
		Packages:  []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}},
	})
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "1 package is newer in your feeds") {
		t.Errorf("a single upgradable package should read as one:\n%s", body)
	}
	if !strings.Contains(body, "2.91-r3 → 2.93-r1") {
		t.Errorf("the manifest should state the one transition:\n%s", body)
	}
}

// TestMaintenanceUpdatesEverythingCurrent: nothing to install is its own quiet
// statement with the age of the answer, not an empty space.
func TestMaintenanceUpdatesEverythingCurrent(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, updatecheck.Truth{
		CheckedAt: time.Now().Add(-90 * time.Minute),
		Firmware:  openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933-4ccb782af7", Server: "https://sysupgrade.mono.si"},
	})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		"Every installed package is the newest version",
		"Checked", "1 h ago",
		"This router runs the newest build its update server offers.",
		"25.12.4 r32933-4ccb782af7",
		"https://sysupgrade.mono.si",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the up-to-date Updates section is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Update now") {
		t.Error("nothing upgradable means no install offer")
	}
}

// TestMaintenanceFirmwareRungs: every rung the check can land on says its own
// plain sentence, and the one that is fixable offers the fix.
func TestMaintenanceFirmwareRungs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		firmware openwrt.FirmwareUpdate
		want     []string
		absent   string
	}{
		{
			name:     "an available build",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051", Server: "https://sysupgrade.openwrt.org", Packages: 78},
			want:     []string{"OpenWrt 25.12.5 r33051 is available for this router", "Available build", "78"},
			// The truth is stated; installing it is not offered from the answer —
			// the only route to a new image stays the manual upload below.
			absent: "Download and install",
		},
		{
			name:     "no upgrade tool",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoOwut, Message: "owut is not installed on this device"},
			want:     []string{"owut", "is not installed", `href="/system/packages/discover?q=owut"`},
		},
		{
			name:     "no server",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoServer, Server: "https://sysupgrade.mono.si", Message: "uclient error code=-1"},
			want:     []string{"No update server answered", "uclient error code=-1"},
		},
		{
			name:     "a device the server cannot build",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUnsupported, Message: "File system type '(null)'"},
			want:     []string{"cannot build an image for this router", "File system type &#39;(null)&#39;"},
		},
		{
			name:     "a check that could not run at all",
			firmware: openwrt.FirmwareUpdate{},
			want:     []string{"The firmware check could not run"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idleUpdates(t)
			s := newServer(t, fakeBackend{access: true})
			knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Firmware: tc.firmware})
			body := get(t, s, "/system/maintenance").Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Errorf("the %s rung is missing %q:\n%s", tc.name, want, body)
				}
			}
			if tc.absent != "" && strings.Contains(body, tc.absent) {
				t.Errorf("the %s rung should not offer %q", tc.name, tc.absent)
			}
			// The manual image upload is the permanent floor under every rung.
			if !strings.Contains(body, "Drop a sysupgrade image here") {
				t.Errorf("the manual upload should remain available on every rung:\n%s", body)
			}
		})
	}
}

// TestUpdateInstallRunsOnceInTheBackground: the POST answers at once, the run is
// the job's, and a second request while it runs changes nothing.
func TestUpdateInstallRunsOnceInTheBackground(t *testing.T) {
	idleUpdates(t)
	upgrades := 0
	blocked := make(chan struct{})
	backend := blockingUpgradeBackend{
		fakeBackend: fakeBackend{access: true, pkgUpgrades: &upgrades},
		blocked:     blocked,
	}
	s := newServer(t, backend)
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}}})

	if rec := postPlugin(t, s, "/system/maintenance/updates/install", url.Values{}); rec.Code != 303 {
		t.Fatalf("install POST = %d, want an immediate redirect", rec.Code)
	}
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "Installing…") {
		t.Errorf("a running install should say so on the button:\n%s", body)
	}
	postPlugin(t, s, "/system/maintenance/updates/install", url.Values{})
	close(blocked)
	waitForCheck(t)
	if upgrades != 1 {
		t.Errorf("PkgUpgrade ran %d times, want exactly one", upgrades)
	}
}

// TestUpdateInstallStatesItsFailureOnce: an upgrade that fails is reported on the
// next render and then forgotten, like every other detached job's outcome.
func TestUpdateInstallStatesItsFailureOnce(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, pkgUpgradeErr: errors.New("apk refused")})
	postPlugin(t, s, "/system/maintenance/updates/install", url.Values{})
	waitForCheck(t)

	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "Update did not complete") || !strings.Contains(body, "apk refused") {
		t.Errorf("a failed install should be stated once:\n%s", body)
	}
	if strings.Contains(get(t, s, "/system/maintenance").Body.String(), "Update did not complete") {
		t.Error("a failure already seen should not be repeated")
	}
}

// TestDeviceRowNamesTheRouter: the sidebar's foot is the device's own row — its
// hostname, the release it runs, and the way to everything that maintains it.
func TestDeviceRowNamesTheRouter(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, hn: "gateway"})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{`href="/system/maintenance"`, ">gateway<", ">dev<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the device row is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "/plugins/hostname/") {
		t.Error("the device row should no longer link to a plugin that does not exist")
	}
	if strings.Contains(body, "Update ready") {
		t.Error("a router with no cached update truth should wear no amber mark")
	}
}

// TestDeviceRowMarksAPendingUpdate: the amber dot and its word appear on the truth
// and nowhere else.
func TestDeviceRowMarksAPendingUpdate(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, hn: "gateway"})
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable, To: "25.12.5"}})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "Update ready") || !strings.Contains(body, "bg-amber-500 ring-2") {
		t.Errorf("a pending update should mark the device row:\n%s", body)
	}
}

// TestOverviewSoftwareTileReadsTheRecordedTruth: the home page's tile is the same
// truth as the footer's mark, and the same doorway.
func TestOverviewSoftwareTileReadsTheRecordedTruth(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	if body := get(t, s, "/").Body.String(); !strings.Contains(body, "Installed software") {
		t.Errorf("an unchecked router should claim nothing on the tile:\n%s", body)
	}
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{
		{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
		{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
	}})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "2 packages ready") || !strings.Contains(body, `href="/system/maintenance"`) {
		t.Errorf("the tile should state the count and lead to the maintenance page:\n%s", body)
	}
}

// TestAutocheckRowReadsTheSetting: the switch states what the config says, and a
// config that says nothing reads as off — an unattended path that touches the
// network is opt-in at the system level (ADR-014 §2).
func TestAutocheckRowReadsTheSetting(t *testing.T) {
	for _, tc := range []struct {
		name string
		uci  map[string]map[string]any
		on   bool
	}{
		{name: "an empty config", uci: nil},
		{
			name: "an explicit off",
			uci:  map[string]map[string]any{"verso": {"updates": map[string]any{".type": "updates", "autocheck": "0"}}},
		},
		{
			name: "an explicit on",
			uci:  map[string]map[string]any{"verso": {"updates": map[string]any{".type": "updates", "autocheck": "1"}}},
			on:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idleUpdates(t)
			s := newServer(t, fakeBackend{access: true, uci: tc.uci})
			body := get(t, s, "/system/maintenance").Body.String()
			for _, want := range []string{
				"Check for updates automatically",
				"It installs nothing on its own.",
				">updates.autocheck<",
				`name="autocheck"`,
				`action="/system/maintenance/updates/autocheck"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("the automatic-check row is missing %q:\n%s", want, body)
				}
			}
			// The switch's checkbox carries `checked` only when the option reads 1.
			checked := strings.Contains(body, `value="1" checked name="autocheck"`)
			if checked != tc.on {
				t.Errorf("with %s the switch is on=%v, want %v", tc.name, checked, tc.on)
			}
		})
	}
}

// TestAutocheckRowUnreadableShowsNoSwitch: when Verso's own config cannot be
// read, the row draws no switch. A toggle guessed off for a setting that is
// actually on would state the opposite of what the router does — the one control
// whose purpose is honesty about unattended network activity — and saving from a
// guessed state would write a decision the owner never made (ADR-014 §2). The row
// says why instead, and offers no Save.
func TestAutocheckRowUnreadableShowsNoSwitch(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, uciReadErr: errors.New("rpcd refused")})
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "could not be read just now") {
		t.Errorf("an unreadable setting should say so plainly:\n%s", body)
	}
	if strings.Contains(body, `name="autocheck"`) {
		t.Error("no switch should be drawn when the setting cannot be read")
	}
	if strings.Contains(body, `action="/system/maintenance/updates/autocheck"`) {
		t.Error("no Save form should be offered when the setting cannot be read")
	}
}

// TestAutocheckSaveCreatesTheSection: a fresh install ships an empty config, so
// the first setting an owner changes is also the section's first appearance —
// created named and typed, then written, both staged (ADR-013 §1).
func TestAutocheckSaveCreatesTheSection(t *testing.T) {
	idleUpdates(t)
	var adds []string
	var writes []uciWrite
	s := newServer(t, fakeBackend{access: true, adds: &adds, writes: &writes})

	rec := postPlugin(t, s, "/system/maintenance/updates/autocheck", url.Values{"autocheck": {"1"}})
	if rec.Code != 303 {
		t.Fatalf("autocheck POST = %d, want a redirect back to the page", rec.Code)
	}
	if len(adds) != 1 || adds[0] != "verso updates updates" {
		t.Fatalf("UCIAdd calls = %v, want one named updates section in verso", adds)
	}
	if len(writes) != 1 || writes[0].config != "verso" || writes[0].section != "updates" || writes[0].values["autocheck"] != "1" {
		t.Fatalf("staged writes = %+v, want verso.updates.autocheck = 1", writes)
	}
}

// TestAutocheckSaveReusesTheSection: a config that already has the section gains
// no second one.
func TestAutocheckSaveReusesTheSection(t *testing.T) {
	idleUpdates(t)
	var adds []string
	var writes []uciWrite
	s := newServer(t, fakeBackend{
		access: true, adds: &adds, writes: &writes,
		uci: map[string]map[string]any{"verso": {"updates": map[string]any{".type": "updates", "autocheck": "1"}}},
	})

	postPlugin(t, s, "/system/maintenance/updates/autocheck", url.Values{"autocheck": {"1"}})
	if len(adds) != 0 {
		t.Errorf("an existing section should not be created again: %v", adds)
	}
	if len(writes) != 1 || writes[0].values["autocheck"] != "1" {
		t.Errorf("staged writes = %+v, want the option written once", writes)
	}
}

// TestAutocheckSaveWritesAnExplicitOff: an unchecked switch posts nothing, and
// that silence is written as 0 rather than by clearing the option — an absent
// option is what the next package install seeds back to on (ADR-014 §2).
func TestAutocheckSaveWritesAnExplicitOff(t *testing.T) {
	idleUpdates(t)
	var writes []uciWrite
	var deletes []string
	s := newServer(t, fakeBackend{
		access: true, writes: &writes, deletes: &deletes,
		uci: map[string]map[string]any{"verso": {"updates": map[string]any{".type": "updates", "autocheck": "1"}}},
	})

	postPlugin(t, s, "/system/maintenance/updates/autocheck", url.Values{})
	if len(writes) != 1 || writes[0].values["autocheck"] != "0" {
		t.Fatalf("staged writes = %+v, want verso.updates.autocheck = 0", writes)
	}
	if len(deletes) != 0 {
		t.Errorf("turning the setting off should never clear the option: %v", deletes)
	}
}

// TestAutocheckSaveSpeaksOnce: the setting's outcome — saved, or refused —
// reaches the person on the page the redirect lands on, and only there. One
// session carries both requests, the way a browser does.
func TestAutocheckSaveSpeaksOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend fakeBackend
		want    string
	}{
		{
			name:    "a staged setting",
			backend: fakeBackend{access: true},
			want:    "Use Save &amp; Apply to put the change into effect",
		},
		{
			name:    "a write rpcd refused",
			backend: fakeBackend{access: true, uciErr: errors.New("rpcd refused")},
			want:    "could not be saved",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idleUpdates(t)
			s := newServer(t, tc.backend)
			do := sameSession(t, s)

			if rec := do(http.MethodPost, "/system/maintenance/updates/autocheck", url.Values{"autocheck": {"1"}}); rec.Code != http.StatusSeeOther {
				t.Fatalf("autocheck POST = %d, want a redirect", rec.Code)
			}
			if body := do(http.MethodGet, maintenancePath, nil).Body.String(); !strings.Contains(body, tc.want) {
				t.Errorf("the page the redirect lands on is missing %q:\n%s", tc.want, body)
			}
			if body := do(http.MethodGet, maintenancePath, nil).Body.String(); strings.Contains(body, tc.want) {
				t.Error("an outcome already seen should not be repeated")
			}
		})
	}
}

// sameSession returns a request driver holding one session across calls, so a
// one-shot flash set by a POST is observable on the render that follows it.
func sameSession(t *testing.T, s *Server) func(method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess, _ := s.sessions.get(token)
	return func(method, path string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			form.Set("_csrf", sess.csrf)
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// TestStagedVersoSettingReachesTheCapsule: Verso's own config is declared by the
// shell, so a staged setting counts, reads, and discards like a firewall rule
// (ADR-013 §3).
func TestStagedVersoSettingReachesTheCapsule(t *testing.T) {
	idleUpdates(t)
	var reverts []string
	s := newServer(t, fakeBackend{
		access:  true,
		reverts: &reverts,
		changes: map[string][][]string{"verso": {{"set", "updates", "autocheck", "0"}}},
	})

	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{"1 pending change", "verso: updates.autocheck = 0"} {
		if !strings.Contains(body, want) {
			t.Errorf("the capsule is missing %q:\n%s", want, body)
		}
	}
	if rec := postPlugin(t, s, "/uci/discard", url.Values{}); rec.Code != 200 {
		t.Fatalf("discard = %d, want 200", rec.Code)
	}
	if len(reverts) != 1 || reverts[0] != "verso" {
		t.Errorf("discard reverted %v, want the verso config", reverts)
	}
}

// blockingUpgradeBackend holds PkgUpgrade open so a test can observe the running
// state the way a browser does — mid-run, from another request.
type blockingUpgradeBackend struct {
	fakeBackend
	blocked chan struct{}
}

func (b blockingUpgradeBackend) PkgUpgrade(ctx context.Context, sid string) error {
	<-b.blocked
	return b.fakeBackend.PkgUpgrade(ctx, sid)
}

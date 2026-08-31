// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// idleUpdates puts the device-wide update jobs back to a known empty state. The
// truth is one router's, not one test's, so every test starts from "never checked".
func idleUpdates(t *testing.T) {
	t.Helper()
	reset := func() {
		updateChecks.mu.Lock()
		updateChecks.active, updateChecks.known, updateChecks.failure = false, false, nil
		updateChecks.truth = updateTruth{}
		updateChecks.mu.Unlock()
		packageUpgrade.mu.Lock()
		packageUpgrade.active, packageUpgrade.failure = false, nil
		packageUpgrade.mu.Unlock()
		feedRefresh.mu.Lock()
		feedRefresh.active, feedRefresh.failure = false, nil
		feedRefresh.mu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// knownUpdates installs a completed check's answer directly, so a render test
// exercises the surfaces rather than the background job's timing.
func knownUpdates(truth updateTruth) {
	updateChecks.mu.Lock()
	defer updateChecks.mu.Unlock()
	updateChecks.known, updateChecks.truth = true, truth
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

// TestUpdateCheckFillsTheCacheOnce: an on-demand check reads both lanes in the
// background, answers the browser at once, and refuses a second concurrent run.
func TestUpdateCheckFillsTheCacheOnce(t *testing.T) {
	idleUpdates(t)
	backend := fakeBackend{
		access:        true,
		pkgUpgradable: []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
		firmware:      openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933"},
	}
	s := newServer(t, backend)

	if _, known := updateChecks.state(); known {
		t.Fatal("a fresh router should hold no cached answer")
	}
	rec := postPlugin(t, s, "/system/maintenance/updates/check", url.Values{})
	if rec.Code != 303 {
		t.Fatalf("check POST = %d, want a redirect back to the page", rec.Code)
	}
	waitForCheck(t)

	truth, known := updateChecks.state()
	if !known || len(truth.Packages) != 1 || truth.Packages[0].Name != "verso" {
		t.Fatalf("cache = %+v known=%v", truth, known)
	}
	if truth.Firmware.State != openwrt.FirmwareCurrent {
		t.Errorf("firmware lane = %q, want the check's answer", truth.Firmware.State)
	}
	if !truth.pending() {
		t.Error("an upgradable package is something to install")
	}
	if truth.CheckedAt.IsZero() {
		t.Error("an answer with no timestamp cannot be shown as fresh or stale")
	}
}

// TestUpdateCheckRefusesASecondRun: the check is one act device-wide.
func TestUpdateCheckRefusesASecondRun(t *testing.T) {
	idleUpdates(t)
	blocked := make(chan struct{})
	started := updateChecks.start(func() (updateTruth, error) {
		<-blocked
		return updateTruth{}, nil
	})
	if !started {
		t.Fatal("an idle job should accept the first run")
	}
	if updateChecks.start(func() (updateTruth, error) { return updateTruth{}, nil }) {
		t.Error("a second check should be refused while one runs")
	}
	close(blocked)
	waitForCheck(t)
}

// TestUpdateCheckKeepsTheLastAnswerOnFailure: a failed run states itself once and
// leaves the previous answer standing — a stale truth beats no truth.
func TestUpdateCheckKeepsTheLastAnswerOnFailure(t *testing.T) {
	idleUpdates(t)
	knownUpdates(updateTruth{
		Packages:  []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}},
		CheckedAt: time.Now().Add(-2 * time.Hour),
	})
	s := newServer(t, fakeBackend{access: true, pkgUpgradableErr: errors.New("helper unreachable")})

	postPlugin(t, s, "/system/maintenance/updates/check", url.Values{})
	waitForCheck(t)

	truth, known := updateChecks.state()
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
	knownUpdates(updateTruth{
		CheckedAt: time.Now(),
		Packages: []openwrt.PackageUpgrade{
			{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
			{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
			{Name: "rpcd", Installed: "2025.12.03", Available: "2026.07.19"},
			{Name: "ubus", Installed: "2025.12.02", Available: "2026.06.28"},
			{Name: "uhttpd", Installed: "2025.10.03", Available: "2026.06.16"},
		},
	})
	s := newServer(t, fakeBackend{access: true})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		"Verso 0.0.23 is available", "this router runs 0.0.22",
		"Updating also brings 4 other packages", "Checked", "just now",
		"This router runs 2.91-r3", "2.93-r1", // the tail rows name both versions
		"4 more packages", // the seam holds everything but the lead
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
	knownUpdates(updateTruth{
		CheckedAt: time.Now(),
		Packages:  []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
	})
	body := get(t, newServer(t, fakeBackend{access: true}), "/system/maintenance").Body.String()
	if !strings.Contains(body, "Verso 0.0.23 is available") {
		t.Errorf("Verso's own update should lead by name:\n%s", body)
	}
	if strings.Contains(body, "Updating also brings") || strings.Contains(body, "more package") {
		t.Error("one package brings nothing else and needs no seam")
	}
}

// TestMaintenanceUpdatesSoftwareLaneWithoutVerso: with Verso itself current, the
// set speaks as a count and the first package still leads the card.
func TestMaintenanceUpdatesSoftwareLaneWithoutVerso(t *testing.T) {
	idleUpdates(t)
	knownUpdates(updateTruth{
		CheckedAt: time.Now(),
		Packages:  []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}},
	})
	s := newServer(t, fakeBackend{access: true})
	body := get(t, s, "/system/maintenance").Body.String()
	if !strings.Contains(body, "1 package is newer in your feeds") {
		t.Errorf("a single upgradable package should read as one:\n%s", body)
	}
	if strings.Contains(body, "more package") {
		t.Error("one package needs no seam behind it")
	}
}

// TestMaintenanceUpdatesEverythingCurrent: nothing to install is its own quiet
// statement with the age of the answer, not an empty space.
func TestMaintenanceUpdatesEverythingCurrent(t *testing.T) {
	idleUpdates(t)
	knownUpdates(updateTruth{
		CheckedAt: time.Now().Add(-90 * time.Minute),
		Firmware:  openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933-4ccb782af7", Server: "https://sysupgrade.mono.si"},
	})
	s := newServer(t, fakeBackend{access: true})
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
			knownUpdates(updateTruth{CheckedAt: time.Now(), Firmware: tc.firmware})
			body := get(t, newServer(t, fakeBackend{access: true}), "/system/maintenance").Body.String()
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
	knownUpdates(updateTruth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}}})

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
	knownUpdates(updateTruth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable, To: "25.12.5"}})
	s := newServer(t, fakeBackend{access: true, hn: "gateway"})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "Update ready") || !strings.Contains(body, "bg-amber-500 ring-2") {
		t.Errorf("a pending update should mark the device row:\n%s", body)
	}
}

// TestOverviewSoftwareTileReadsTheCache: the home page's tile is the same truth as
// the footer's mark, and the same doorway.
func TestOverviewSoftwareTileReadsTheCache(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	if body := get(t, s, "/").Body.String(); !strings.Contains(body, "Installed software") {
		t.Errorf("an unchecked router should claim nothing on the tile:\n%s", body)
	}
	knownUpdates(updateTruth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{
		{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
		{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
	}})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "2 packages ready") || !strings.Contains(body, `href="/system/maintenance"`) {
		t.Errorf("the tile should state the count and lead to the maintenance page:\n%s", body)
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

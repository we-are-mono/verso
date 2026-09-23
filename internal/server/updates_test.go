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
		for _, job := range []*backgroundJob{&updateChecks, &packageUpgrade, &firmwareUpgrade, &feedRefresh} {
			job.mu.Lock()
			job.active, job.failure, job.done = false, nil, false
			job.mu.Unlock()
		}
		firmwareTakeoverReleased.Store(false)
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
	for updateChecks.running() || packageUpgrade.running() || firmwareUpgrade.running() {
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

// TestUpdateCheckSpeaksInlineOnly: the firmware section itself turns to
// "Checking for updates…" and back to its verdict, so the check sets no flash —
// a banner on top would say the same thing a second time, somewhere else.
func TestUpdateCheckSpeaksInlineOnly(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	rec, token := postPluginRequest(t, s, "/system/maintenance/updates/check", url.Values{}, nil)
	if rec.Code != 303 {
		t.Fatalf("check POST = %d, want a redirect back to the page", rec.Code)
	}
	waitForCheck(t)
	if _, message := s.sessions.TakeFlash(token); message != "" {
		t.Errorf("the check set a flash %q; its state belongs to the section alone", message)
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
		"Firmware", "Not checked yet",
		"Check again", `action="/system/maintenance/updates/check"`,
		"is not known until it checks",
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
		// The manifest states each transition as three aligned columns: the
		// installed version, a header-less arrow, and the available one.
		">2.91-r3<", ">→<", ">2.93-r1<", ">Installed<", ">Available<",
		"What changes — 5 packages", // the manifest folds until asked
		"Update now", `action="/system/maintenance/updates/install"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the software lane is missing %q:\n%s", want, body)
		}
	}
	// Verso rides the manifest as a row like any other — no card leads it.
	if strings.Contains(body, "This router runs 0.0.22") {
		t.Errorf("the verso version card should be gone:\n%s", body)
	}
	// The masthead is the announcement: a waiting update retitles the page in the
	// action colour, and the navigation suffix steps aside — a message, not a
	// place-label.
	for _, want := range []string{"Maintenance", "text-denim-deep"} {
		if !strings.Contains(body, want) {
			t.Errorf("the news masthead is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, " — Maintenance</span>") {
		t.Errorf("a toned masthead should drop the navigation suffix:\n%s", body)
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
	for _, want := range []string{">2.91-r3<", ">→<", ">2.93-r1<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the manifest should state the one transition, missing %q:\n%s", want, body)
		}
	}
}

// TestMaintenanceUpdatesEverythingCurrent: both halves current opens the section
// with one confirmation box, and the package lane does not render at all — its
// only content would repeat what the box just said. The firmware lane stays for
// its facts, which are not repetition.
func TestMaintenanceUpdatesEverythingCurrent(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	knownUpdates(t, s, updatecheck.Truth{
		CheckedAt: time.Now().Add(-90 * time.Minute),
		Firmware:  openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933-4ccb782af7", Server: "https://sysupgrade.mono.si"},
	})
	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		"Up to date",
		"Check again",
		"Checked", "1 h ago",
		"This router runs the newest build its update server offers.",
		// The ledger splits the build into its version and its revision, and
		// names the server it was checked against.
		">Current<", ">25.12.4<", "r32933-4ccb782af7",
		"Checked against", "https://sysupgrade.mono.si",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the up-to-date Updates section is missing %q:\n%s", want, body)
		}
	}
	// Nothing is offered, so the ledger has no Available column; nothing is
	// wrong, so the verdict stands plain, not in the warning band.
	if strings.Contains(body, ">Available<") || strings.Contains(body, "data-verso-ledger-warning") {
		t.Errorf("a current router's ledger should have no Available column and no warning band:\n%s", body)
	}
	if strings.Contains(body, "Every installed package is the newest version") {
		t.Error("the resting package lane should not render beside the verdict box")
	}
	if strings.Contains(body, "Update now") {
		t.Error("nothing upgradable means no install offer")
	}
	// The resting rung states one build, so the version pair's comparison tone
	// has nothing to compare: a lone tinted version would warn about nothing.
	for _, unwanted := range []string{"font-semibold text-marigold-deep", "font-semibold text-green-deep", "Download and install"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the resting firmware rung should not carry %q:\n%s", unwanted, body)
		}
	}
	// At rest the masthead is the ordinary place-label, suffix and all.
	if strings.Contains(body, "An update is ready") {
		t.Error("a current router announces nothing")
	}
	if !strings.Contains(body, ">Maintenance</h1>") {
		t.Errorf("the resting masthead should name Maintenance:\n%s", body)
	}
}

// TestMaintenanceFirmwareRungs: every rung the check can land on says its own
// plain sentence, and the one that is fixable offers the fix.
func TestMaintenanceFirmwareRungs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		firmware  openwrt.FirmwareUpdate
		want      []string
		absent    string
		inWarning string // must sit inside the warning band, not beside it
	}{
		{
			name:     "an available build",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051", Server: "https://sysupgrade.openwrt.org", Packages: 78},
			want: []string{
				// The verdict names the build on offer…
				"25.12.5 is available", "The update server is offering a newer build for this router.",
				// …and the ledger sets it beside what runs, the new value marked
				// by the packet rather than by a tinted cell.
				">Current<", ">Available<", ">25.12.4<", "r32933", ">25.12.5<", "r33051",
				`data-verso-ledger-changed`,
				"Built by", "https://sysupgrade.openwrt.org", "78 packages change",
				// Installing is caution, not danger: the same one-hue confirm, in
				// marigold, naming the build.
				"Download and install 25.12.5", `data-verso-confirm-tone="caution"`, "Install 25.12.5 now?",
				`action="/system/maintenance/updates/firmware"`,
				"restart on its own", "Do not disconnect its power.",
				"or upload a custom image…",
			},
			// The old facts list and its toned version pair are gone.
			absent: "Installed build",
		},
		{
			name:     "no upgrade tool",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoOwut, Message: "owut is not installed on this device"},
			want:     []string{`data-verso-ledger-warning`, "owut", "is not installed", `href="/system/packages/discover?q=owut"`},
		},
		{
			name:     "no server",
			firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoServer, Server: "https://sysupgrade.mono.si", Message: "uclient error code=-1"},
			// A warning, not an error: the verdict stands in the marigold band,
			// and the tool's own words ride inside it, verbatim and copyable.
			want:      []string{`data-verso-ledger-warning`, "The update server didn&#39;t answer", "No update server answered", `data-verso-ledger-complaint`, "uclient error code=-1"},
			inWarning: "uclient error code=-1",
		},
		{
			name:      "a device the server cannot build",
			firmware:  openwrt.FirmwareUpdate{State: openwrt.FirmwareUnsupported, Message: "File system type '(null)'"},
			want:      []string{`data-verso-ledger-warning`, "No firmware updates for this router", "cannot build an image for this router", `data-verso-ledger-complaint`, "File system type &#39;(null)&#39;"},
			inWarning: "File system type &#39;(null)&#39;",
		},
		{
			name:     "a check that could not run at all",
			firmware: openwrt.FirmwareUpdate{},
			want:     []string{`data-verso-ledger-warning`, "The firmware check could not run"},
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
			if tc.inWarning != "" {
				if band := section(t, body, "data-verso-ledger-warning", "<table"); !strings.Contains(band, tc.inWarning) {
					t.Errorf("the %s rung should carry %q inside its warning band:\n%s", tc.name, tc.inWarning, band)
				}
				if strings.Contains(body, "Error, given by the update server") {
					t.Errorf("the %s rung should not label the tool's words separately", tc.name)
				}
			}
			// The manual image upload is the permanent floor under every rung —
			// including the one that now offers the act, because a server that
			// can build for this device is not the only way in.
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
	if !strings.Contains(body, "Package update failed") || !strings.Contains(body, "apk refused") {
		t.Errorf("a failed install should be stated once:\n%s", body)
	}
	if strings.Contains(get(t, s, "/system/maintenance").Body.String(), "Package update failed") {
		t.Error("a failure already seen should not be repeated")
	}
}

// TestFirmwareUpgradeRunsOnceInTheBackground: the POST answers at once — the act
// is a build on someone else's server followed by a flash, so there is nothing to
// hold a browser for — and while it runs the maintenance area is the full-screen
// takeover, chrome-less and server-authoritative, so a reload lands back on it.
// A second start while one runs changes nothing: the helper's guard keeps the two
// apart and the browser lands on the same takeover.
func TestFirmwareUpgradeRunsOnceInTheBackground(t *testing.T) {
	idleUpdates(t)
	upgrades := 0
	blocked := make(chan struct{})
	backend := blockingFirmwareBackend{
		fakeBackend: fakeBackend{access: true, firmwareUpgrades: &upgrades},
		blocked:     blocked,
	}
	s := newServer(t, backend)
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{
		State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051",
	}})

	if rec := postPlugin(t, s, "/system/maintenance/updates/firmware", url.Values{}); rec.Code != 303 {
		t.Fatalf("firmware install POST = %d, want an immediate redirect", rec.Code)
	}
	body := get(t, s, "/system/maintenance").Body.String()
	// The running job paints the takeover's "preparing" state, chrome-less.
	for _, want := range []string{
		`data-verso-upgrading`, `data-verso-upgrading-state="preparing"`,
		"Preparing your new firmware", "Keep the router powered",
		"returns to its current version on its own",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a running firmware install should paint the takeover, missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `x-data="sidebar"`) || strings.Contains(body, "<aside") {
		t.Errorf("the takeover is chrome-less — no sidebar/nav:\n%s", body)
	}
	postPlugin(t, s, "/system/maintenance/updates/firmware", url.Values{})
	close(blocked)
	waitForCheck(t)
	if upgrades != 1 {
		t.Errorf("FirmwareUpgrade ran %d times, want exactly one", upgrades)
	}
}

// TestFirmwareUpgradeFailureHoldsTheTakeover: an upgrade owut refused stops the
// motion and shows the truth — the plain meaning in the callout, the tool's own
// words in the labelled box, and the two doors out. Unlike every other detached
// job's outcome it is NOT forgotten on the next visit: the takeover is
// server-authoritative until the person releases it, so a reload keeps landing on
// the failure. "Back to Maintenance" is the release, and only then does the
// ordinary page return.
func TestFirmwareUpgradeFailureHoldsTheTakeover(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, firmwareUpgradeErr: errors.New("Update checks reveal errors, can't proceed")})
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{
		State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051",
	}}) // an available update is the precondition to install one
	postPlugin(t, s, "/system/maintenance/updates/firmware", url.Values{})
	waitForCheck(t)

	body := get(t, s, "/system/maintenance").Body.String()
	for _, want := range []string{
		`data-verso-upgrading-state="failed"`,
		"The upgrade couldn&#39;t start", "still on the firmware it started with",
		"Error, given by the update server", "Update checks reveal errors, can&#39;t proceed",
		"Back to Maintenance", "Try again",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a failed firmware install should hold the takeover, missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `x-data="sidebar"`) || strings.Contains(body, "<aside") {
		t.Errorf("the failed takeover is chrome-less — no sidebar/nav:\n%s", body)
	}
	// Server-authoritative: a second visit still lands on the failure.
	if !strings.Contains(get(t, s, "/system/maintenance").Body.String(), `data-verso-upgrading-state="failed"`) {
		t.Error("an unacknowledged failure must persist, not be forgotten")
	}
	// The release returns the ordinary page.
	if rec := postPlugin(t, s, "/system/maintenance/updates/firmware/dismiss", url.Values{}); rec.Code != 303 {
		t.Fatalf("dismiss POST = %d, want a redirect back to the page", rec.Code)
	}
	after := get(t, s, "/system/maintenance").Body.String()
	if strings.Contains(after, "data-verso-upgrading") {
		t.Errorf("after the release the ordinary page returns, not the takeover:\n%s", after)
	}
	if !strings.Contains(after, `x-data="sidebar"`) {
		t.Errorf("the ordinary maintenance page carries the shell chrome:\n%s", after)
	}
}

// TestSidebarFootCarriesNoDeviceRow: the nav's foot marks no update and links
// no device page — the update state lives on the pages that own it, and the
// device's name is the corner nameplate, not a row.
func TestSidebarFootCarriesNoDeviceRow(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true, hn: "gateway"})
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable, To: "25.12.5"}})
	body := get(t, s, "/system/services").Body.String()
	if strings.Contains(body, "Update ready") {
		t.Error("the nav chrome should not carry the update mark")
	}
	if strings.Contains(body, `class="group relative flex items-center gap-3 rounded-xl`) {
		t.Errorf("the sidebar foot should carry no device row:\n%s", body)
	}
}

// TestNameplateWearsTheHostname: the top bar is the device's, not the
// software's — the hostname links home, on one line whatever its length, with
// the whole name a hover away where the bar has to cut it; and a box with no
// readable name says what it is instead of pretending a brand.
func TestNameplateWearsTheHostname(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, hn: "jedis-are-not-as-great-as-sith"})
	body := get(t, s, "/system/services").Body.String()
	if !strings.Contains(body, ">jedis-are-not-as-great-as-sith</a>") {
		t.Errorf("the bar should wear the hostname:\n%s", body)
	}
	if !strings.Contains(body, `title="jedis-are-not-as-great-as-sith" class="min-w-0 truncate font-mono`) {
		t.Error("a long name stays on one line, and the whole of it is a hover away")
	}

	nameless := newServer(t, fakeBackend{access: true})
	if body := get(t, nameless, "/system/services").Body.String(); !strings.Contains(body, ">This device</a>") {
		t.Errorf("a box with no readable hostname should say what it is:\n%s", body)
	}
}

// The landing-page header reports only recorded updates and links to maintenance.
func TestOverviewHeaderReadsTheRecordedTruth(t *testing.T) {
	idleUpdates(t)
	s := newServer(t, fakeBackend{access: true})
	if body := get(t, s, "/").Body.String(); strings.Contains(body, "package update available") {
		t.Errorf("an unchecked router should claim nothing on the tile:\n%s", body)
	}
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{
		{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
		{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
	}})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "2 package updates available") || !strings.Contains(body, `href="/system/maintenance"`) {
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
				`name="autocheck"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("the automatic-check row is missing %q:\n%s", want, body)
				}
			}
			// The switch's checkbox carries `checked` only when the option reads 1.
			tag := body[strings.Index(body, `id="autocheck"`):]
			checked := strings.Contains(tag[:strings.Index(tag, ">")], " checked")
			if checked != tc.on {
				t.Errorf("with %s the switch is on=%v, want %v", tc.name, checked, tc.on)
			}
			// The switch stands outside any form and posts itself; a Save
			// button for one bit would be furniture.
			if strings.Contains(body, ">Save</button>") {
				t.Errorf("the automatic-check row should offer no Save button:\n%s", body)
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
	if strings.Contains(body, ">Save</button>") {
		t.Error("no Save should be offered when the setting cannot be read")
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

	rec := postPlugin(t, s, "/system/maintenance", url.Values{"autocheck": {"on"}})
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

	postPlugin(t, s, "/system/maintenance", url.Values{"autocheck": {"on"}})
	if len(adds) != 0 {
		t.Errorf("an existing section should not be created again: %v", adds)
	}
	if len(writes) != 1 || writes[0].values["autocheck"] != "1" {
		t.Errorf("staged writes = %+v, want the option written once", writes)
	}
}

// TestAutocheckSaveWritesAnExplicitOff: the self-posting switch states its new
// position outright, and "off" is written as 0 rather than by clearing the
// option — an absent option is what the next package install seeds back to on
// (ADR-014 §2).
func TestAutocheckSaveWritesAnExplicitOff(t *testing.T) {
	idleUpdates(t)
	var writes []uciWrite
	var deletes []string
	s := newServer(t, fakeBackend{
		access: true, writes: &writes, deletes: &deletes,
		uci: map[string]map[string]any{"verso": {"updates": map[string]any{".type": "updates", "autocheck": "1"}}},
	})

	postPlugin(t, s, "/system/maintenance", url.Values{"autocheck": {"off"}})
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
			want:    "Saved. Nothing is live until you apply.",
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

			if rec := do(http.MethodPost, "/system/maintenance", url.Values{"autocheck": {"on"}}); rec.Code != http.StatusSeeOther {
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

// TestStagedVersoSettingReachesTheChip: Verso's own config is declared by the
// shell, so a staged setting counts, reads, and discards like a firewall rule
// (ADR-013 §3).
func TestStagedVersoSettingReachesTheChip(t *testing.T) {
	idleUpdates(t)
	var reverts []string
	s := newServer(t, fakeBackend{
		access:  true,
		reverts: &reverts,
		changes: map[string][][]string{"verso": {{"set", "updates", "autocheck", "0"}}},
	})

	// The chip counts it on the page; the humanized line waits in the drawer,
	// under System, since the config is the shell's own.
	if body := get(t, s, "/system/maintenance").Body.String(); !strings.Contains(body, ">1 staged change</span>") {
		t.Errorf("the chip is missing the staged setting:\n%s", body)
	}
	drawer := getPanel(t, s, "/uci/review").Body.String()
	for _, want := range []string{`text-body">System</span>`, "verso: updates.autocheck = 0"} {
		if !strings.Contains(drawer, want) {
			t.Errorf("the drawer is missing %q:\n%s", want, drawer)
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

// blockingFirmwareBackend does the same for the firmware act, which on a real
// device is minutes long: the page has to be readable while it runs.
type blockingFirmwareBackend struct {
	fakeBackend
	blocked chan struct{}
}

func (b blockingFirmwareBackend) FirmwareUpgrade(ctx context.Context, sid string) error {
	<-b.blocked
	return b.fakeBackend.FirmwareUpgrade(ctx, sid)
}

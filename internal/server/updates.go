// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
	"github.com/we-are-mono/verso/internal/widget"
)

// Updates are two lanes with one shape: packages, which apk answers for uniformly
// on every device, and the system firmware, which an attended-sysupgrade server
// answers for — or honestly cannot. Both truths are read away from any render:
// the package feeds are remote, and the firmware check talks to a server over the
// internet, so a page that asked either question while rendering would be a page
// that hangs on someone else's network.
//
// The answer lives in a file (ADR-014 §5), not in this process. The daily cron run
// writes it as local root and the Check-for-updates button writes it as the
// operator, through the one shared writer in internal/updatecheck; every surface
// here reads that file and nothing else. So the truth survives a shell restart,
// states its own age, and — the point of the whole arrangement — is there for an
// owner who never went looking. Before any check there is no file, and the
// surfaces that exist to nudge simply do not render.

// Every update surface lands on one page, and the software itself on one other.
const (
	maintenancePath = "/system/maintenance"
	packagesPath    = "/system/packages"
)

// updateChecks is the one check allowed to run at a time, and packageUpgrade and
// firmwareUpgrade the two installations it leads to. All three are device-wide:
// what a router can install is a property of the router, not of whoever is
// looking at it. None holds the answer — only whether a run is under way, and how
// the last one failed.
var (
	updateChecks    backgroundJob
	packageUpgrade  backgroundJob
	firmwareUpgrade backgroundJob
)

// firmwareTakeoverReleased is set when a person leaves the takeover after its
// countdown ran out — the one exit from a run that has taken far longer than a
// whole upgrade should. It lets the takeover stand down even while the background
// run continues (a stalled build, or a router that never came back), so the
// screen is never a trap. A fresh upgrade clears it.
var firmwareTakeoverReleased atomic.Bool

// firmwareInstallPath is where the firmware act posts — a peer of the check and
// the package install, under the one page that owns all three.
const firmwareInstallPath = maintenancePath + "/updates/firmware"

// updateTruth is the recorded answer and whether there is one at all — the single
// accessor the three surfaces share, so they can never disagree about what this
// router knows.
func (s *Server) updateTruth() (updatecheck.Truth, bool) {
	return updatecheck.Read(s.stateDir)
}

// startUpdateCheck reads both lanes under the operator's session and records the
// result where every surface — and the next shell process — will find it. The run
// outlives the request that began it, so it carries a context of its own; the
// helper client's deadline is what bounds each call. Failing to record the answer
// is failing the check: an answer nobody can read is not one.
func (s *Server) startUpdateCheck(sid string) bool {
	return updateChecks.start(func() error {
		truth, err := updatecheck.Run(context.Background(), s.backend, sid)
		if err != nil {
			return err
		}
		return updatecheck.Write(s.stateDir, truth)
	})
}

// startPackageUpgrade installs every upgradable package in the background. apk
// holds verso-rpcd's package guard for the whole run, so this can never be the
// browser's wait; the job's own guard is what keeps two runs apart.
func (s *Server) startPackageUpgrade(sid string) bool {
	return packageUpgrade.start(func() error {
		if err := s.backend.PkgUpgrade(context.Background(), sid); err != nil {
			return err
		}
		// An upgrade can replace a plugin package, so the manifests are re-read the
		// way an install does it (ADR-011 §7); and what was upgradable a moment ago
		// no longer is, so the truth is re-read too — the page that reports the
		// outcome reports the new state, not the old one.
		s.rescanManifests()
		s.startUpdateCheck(sid)
		return nil
	})
}

// startFirmwareUpgrade has the helper build, download and verify a new image for
// this device and then start the install. The helper holds both of its guards for
// the whole run, so no package work can overlap it; this job's guard is what keeps
// two of these apart and what lets every surface say the router is busy with one.
//
// The run does not end when the router is running the new build — it ends when the
// image is on the device and sysupgrade has been handed it. Moments later that
// sysupgrade takes this process down with the rest of userspace, so success here is
// "the flash has begun", and nothing renders after it.
func (s *Server) startFirmwareUpgrade(sid string) bool {
	started := firmwareUpgrade.start(func() error {
		return s.backend.FirmwareUpgrade(context.Background(), sid)
	})
	if started {
		// A fresh run owns the screen again — clear any release the previous run's
		// countdown left behind.
		firmwareTakeoverReleased.Store(false)
	}
	return started
}

// handleUpdatesCheck starts an on-demand check. It answers immediately: the run is
// the background job's. It says nothing through the flash, started or already
// running: the Check again button itself reads "Checking…" while the run lasts,
// and the page reloads onto the new verdict when it ends.
func (s *Server) handleUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	s.startUpdateCheck(s.sessionSID(r))
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}

// handleUpdatesInstall starts the package upgrade and answers at once. A second
// request while one runs changes nothing and says so.
func (s *Server) handleUpdatesInstall(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	switch {
	case feedRefresh.running():
		s.flash(r, "info", tr("The feeds are being refreshed — try again in a moment."))
	case !s.startPackageUpgrade(s.sessionSID(r)):
		s.flash(r, "info", tr("The updates are already being installed."))
	default:
		s.flash(r, "info", tr("Installing updates — reload in a moment to see the result."))
	}
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}

// handleUpdatesFirmware starts the firmware upgrade and answers at once, sending
// the browser back to Maintenance — which, with the upgrade now under way, is the
// full-screen takeover (handleSystemMaintenance). What follows is a build on the
// update server, a download, and then a flash — minutes in which this router has
// nothing to report but that the act began, and at the end of which it is not
// answering at all. The takeover states all of that itself, so a flash the
// chrome-less page would never show is not set; only the one refusal that keeps
// the ordinary page — software already installing — speaks through the flash.
//
// A start that finds a run already under way changes nothing and needs no word:
// the redirect lands on the same takeover either way. This is also the door
// "Try again" re-enters after a failure — start clears the spent outcome and
// begins afresh.
func (s *Server) handleUpdatesFirmware(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	switch {
	case feedRefresh.running() || packageUpgrade.running():
		s.flash(r, "info", tr("Software is being installed right now — try again when that finishes."))
	case !firmwareUpgrade.running() && !s.firmwareUpdateAvailable():
		// Nothing to install: the recorded check offers no newer build. A run here
		// would only hand owut a build that does not exist and fail on the server;
		// a run already under way is left to speak through the takeover, so it is
		// exempt from this gate.
		s.flash(r, "info", tr("There is no firmware update to install right now."))
	default:
		s.startFirmwareUpgrade(s.sessionSID(r))
	}
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}

// firmwareUpdateAvailable reports whether the recorded check offers a newer build
// to install — the guard the firmware act reads before it commits the router to a
// flash. Unknown (never checked) reads as unavailable: without a recorded target
// there is nothing to install.
func (s *Server) firmwareUpdateAvailable() bool {
	truth, known := s.updateTruth()
	return known && truth.Firmware.State == openwrt.FirmwareUpdateAvailable
}

// autocheckLane is the standing arrangement under the two answers: whether this
// router goes looking on its own, so an owner who never opens this page still
// learns that an update exists (ADR-014). The switch reads its state from uci
// through rpcd, and saving it stages like every other setting.
func (s *Server) autocheckLane(ctx context.Context, sid string) widget.Widget {
	value, readable := s.versoOption(ctx, sid, updatesSectionName, autocheckOption)
	if !readable {
		// The setting could not be read, so there is no honest switch to draw and
		// no state to save from. Say why rather than show a toggle that might state
		// the opposite of what the router does — and offer no Save, so a guessed
		// value is never written.
		return &widget.Callout{Variant: "neutral", Compact: true,
			Body: "Whether this router checks for updates on its own could not be read just now. Reload in a moment."}
	}
	// The switch stands outside any form, so flipping it is a complete
	// instruction (ADR-010): the shell's client posts it to this page, the
	// flip stages, and the staged-changes chip appears because a pending change now
	// exists — no Save button for one bit, no standing bar either.
	return &widget.Settings{Items: []widget.SettingsItem{{
		Title: "Check for updates automatically",
		Desc:  "Once a day this router asks what is available. It installs nothing on its own.",
		Code:  updatesSectionName + "." + autocheckOption,
		Toggle: &widget.SettingsToggle{
			Name: autocheckOption,
			On:   value == optionOn,
		},
	}}}
}

// handleUpdatesAutocheck stages the daily check's gate. The flip arrives from
// the shell's self-posting switch, which states the new position outright —
// "on" or "off" — so exactly "on" means yes and everything else means no,
// written as an explicit 0 rather than by clearing the option, because an
// absence is what the next package install seeds back to on (ADR-014 §2).
func (s *Server) handleUpdatesAutocheck(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	value := optionOff
	if r.PostForm.Get(autocheckOption) == "on" {
		value = optionOn
	}
	if err := s.stageVersoOption(r.Context(), s.sessionSID(r), updatesSectionName, autocheckOption, value); err != nil {
		log.Printf("verso: updates: staging the automatic check failed: %v", err)
		s.flash(r, "danger", tr("That setting could not be saved just now. Try again in a moment."))
	} else {
		s.flash(r, "info", tr("Saved. Nothing is live until you apply."))
	}
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}

// softwareLane is the package half: what the feeds hold newer copies of, Verso's
// own package leading, and the one act that installs them all.
func softwareLane(truth updatecheck.Truth, known, checking bool) widget.Widget {
	section := &widget.Section{Title: "Software", Hairline: true}
	switch {
	case !known:
		section.Sub = "Which packages have newer versions in your feeds is not known until this router checks."
		return section
	case len(truth.Packages) == 0:
		section.Sub = "Every installed package is the newest version your feeds offer."
		return section
	}
	section.Sub = softwareLead(truth)
	if manifest := upgradableList(truth); manifest != nil {
		section.Children = append(section.Children, manifest)
	}
	section.Children = append(section.Children,
		&widget.Form{Action: maintenancePath + "/updates/install", NoSubmit: true, Fields: []widget.Widget{
			updatesInstallButton(checking),
		}})
	return section
}

// softwareLead names the update a person came for. Verso's own package is that one
// whenever it is in the set; otherwise the set speaks as a count.
func softwareLead(truth updatecheck.Truth) string {
	verso, ok := truth.Verso()
	if !ok {
		return packagesAre(len(truth.Packages)) + " newer in your feeds than what this router runs."
	}
	lead := fmt.Sprintf("**Verso %s is available** — this router runs %s.", verso.Available, verso.Installed)
	if others := len(truth.Packages) - 1; others > 0 {
		return lead + " Updating also brings " + otherPackages(others) + "."
	}
	return lead
}

func packagesAre(n int) string {
	if n == 1 {
		return "1 package is"
	}
	return strconv.Itoa(n) + " packages are"
}

func otherPackages(n int) string {
	if n == 1 {
		return "1 other package"
	}
	return strconv.Itoa(n) + " other packages"
}

// upgradableList is the manifest of what the update would change, wearing the
// conditions-card surface and folded until asked — the lane's sentence carries
// the news, the summary line carries the count, and the table answers the
// person who wants the detail. Verso's own package is a row in it like any
// other, and the version change reads as three fit columns (installed, a
// header-less arrow, available) huddled at the right edge, so the befores, the
// arrows, and the afters each align. Verso alone needs no manifest: the lane's
// lead sentence is the whole story.
func upgradableList(truth updatecheck.Truth) widget.Widget {
	if _, ok := truth.Verso(); ok && len(truth.Packages) == 1 {
		return nil
	}
	rows := make([]widget.TableRow, 0, len(truth.Packages))
	for _, p := range truth.Packages {
		rows = append(rows, widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
			{Text: p.Name},
			{Text: p.Installed},
			{Text: "→"},
			{Text: p.Available},
		}})
	}
	return &widget.Disclosure{
		Style:   "condition",
		Summary: changesSummary(len(truth.Packages)),
		Children: []widget.Widget{&widget.Table{
			Dense: true,
			Columns: []widget.TableColumn{
				{Label: "Package", Kind: "name"},
				{Label: "Installed", Kind: "mono", Fit: true},
				{Kind: "keyword", Fit: true},
				{Label: "Available", Kind: "mono", Fit: true},
			},
			Rows: rows,
		}},
	}
}

func changesSummary(n int) string {
	if n == 1 {
		return "What changes — 1 package"
	}
	return "What changes — " + strconv.Itoa(n) + " packages"
}

func updatesInstallButton(checking bool) *widget.Button {
	button := &widget.Button{Label: "Update now", Style: "primary", Icon: "download", Name: "action", Value: "install"}
	switch {
	case packageUpgrade.running():
		button.Label, button.Loading = "Installing…", true
	case feedRefresh.running() || checking || firmwareUpgrade.running():
		button.Disabled = true
	}
	return button
}

// firmwareInstallAct is the act an offered build leads to. It is caution, not
// danger: the router replaces its operating system and is gone for minutes, but
// that is the point of the act, and a slot it cannot boot falls back on its own.
// So it asks in marigold, naming the build. With the run under way there is
// nothing left to confirm, so the control states what is happening; while apk or
// the check owns the helper's package guard there is nothing to start, so it says
// that by being unavailable rather than by failing after the press. The labels
// arrive localized, because they carry the version.
func firmwareInstallAct(tr func(string) string, checking bool, version string) widget.Widget {
	trigger := fmt.Sprintf(tr("Download and install %s"), version)
	var control widget.Widget
	switch {
	case firmwareUpgrade.running():
		control = &widget.Button{Label: "Installing…", Style: "primary", Icon: "download", Loading: true, Name: "action", Value: "install"}
	case packageUpgrade.running() || feedRefresh.running() || checking:
		control = &widget.Button{Label: trigger, Style: "primary", Icon: "download", Disabled: true, Name: "action", Value: "install"}
	default:
		control = &widget.Confirm{
			Tone:    widget.ToneCaution,
			Trigger: trigger,
			Title:   fmt.Sprintf(tr("Install %s now?"), version),
			Message: "The router will fetch the new build, install it, and restart on its own. It will be unavailable for several minutes. Do not disconnect its power.",
			Confirm: "Install firmware", Cancel: "Not yet",
		}
	}
	return &widget.Form{Action: firmwareInstallPath, NoSubmit: true, Fields: []widget.Widget{control}}
}

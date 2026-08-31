// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// Updates are two lanes with one shape: packages, which apk answers for uniformly
// on every device, and the system firmware, which an attended-sysupgrade server
// answers for — or honestly cannot. Both truths are read in the background and
// cached: the package feeds are remote, and the firmware check talks to a server
// over the internet, so a page that asked either question while rendering would be
// a page that hangs on someone else's network. Every surface reads this cache and
// nothing else; before the first check there is no cache, and the surfaces that
// exist to nudge simply do not render.

// versoPackage is Verso's own package name. It leads the software lane, because an
// update to the thing the person is looking at is the one they came to read about.
const versoPackage = "verso"

// Every update surface lands on one page, and the software itself on one other.
const (
	maintenancePath = "/system/maintenance"
	packagesPath    = "/system/packages"
)

// updateTruth is one complete answer from both lanes, and the moment it was read.
type updateTruth struct {
	Packages  []openwrt.PackageUpgrade
	Firmware  openwrt.FirmwareUpdate
	CheckedAt time.Time
}

// pending reports whether the device has anything to install — the one boolean the
// homepage tile and the sidebar's device row are allowed to draw attention with.
func (t updateTruth) pending() bool {
	return len(t.Packages) != 0 || t.Firmware.State == openwrt.FirmwareUpdateAvailable
}

// verso returns Verso's own entry in the upgradable set, if it is in it.
func (t updateTruth) verso() (openwrt.PackageUpgrade, bool) {
	for _, p := range t.Packages {
		if p.Name == versoPackage {
			return p, true
		}
	}
	return openwrt.PackageUpgrade{}, false
}

// updateCheckJob holds the cached truth and the one background run allowed to
// refresh it. It follows the shape of the other device-wide jobs, and keeps the
// answer as well as the failure: what a check found is what every surface reads.
type updateCheckJob struct {
	mu      sync.Mutex
	active  bool
	known   bool
	truth   updateTruth
	failure error
}

// start reads both truths in the background unless a run is already under way,
// reporting whether this call owns the new one. A failed run leaves the previous
// answer in place: a stale truth with its own timestamp beats no truth at all.
func (j *updateCheckJob) start(check func() (updateTruth, error)) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active {
		return false
	}
	j.active = true
	j.failure = nil
	go func() {
		truth, err := check()
		j.mu.Lock()
		j.active, j.failure = false, err
		if err == nil {
			j.known, j.truth = true, truth
		}
		j.mu.Unlock()
	}()
	return true
}

func (j *updateCheckJob) running() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active
}

// state is the cached answer and whether there is one at all.
func (j *updateCheckJob) state() (updateTruth, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.truth, j.known
}

// takeFailure reports how the last finished run failed and forgets it, so a
// failure nobody was waiting for is still stated once, on the next visit.
func (j *updateCheckJob) takeFailure() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	failure := j.failure
	j.failure = nil
	return failure
}

// updateChecks is that job, and packageUpgrade the installation it leads to. Both
// are device-wide: what a router can install is a property of the router, not of
// whoever is looking at it.
var (
	updateChecks   updateCheckJob
	packageUpgrade backgroundJob
)

// startUpdateCheck reads both lanes under the operator's session. The run outlives
// the request that began it, so it carries a context of its own; the helper
// client's deadline is what bounds each call.
//
// The package lane failing is the check failing — apk answers on every device. The
// firmware lane failing is not: the helper reports a rung rather than an error for
// every device it cannot answer for, so a transport failure here leaves the lane's
// state empty and the page says the check could not run.
func (s *Server) startUpdateCheck(sid string) bool {
	return updateChecks.start(func() (updateTruth, error) {
		ctx := context.Background()
		packages, err := s.backend.PkgUpgradable(ctx, sid)
		if err != nil {
			return updateTruth{}, err
		}
		firmware, err := s.backend.FirmwareCheck(ctx, sid)
		if err != nil {
			log.Printf("verso: updates: firmware check unavailable: %v", err)
			firmware = openwrt.FirmwareUpdate{}
		}
		return updateTruth{Packages: packages, Firmware: firmware, CheckedAt: time.Now()}, nil
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

// handleUpdatesCheck starts an on-demand check. It answers immediately: the run is
// the background job's, and the next render is what shows its result.
func (s *Server) handleUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if s.startUpdateCheck(s.sessionSID(r)) {
		s.flash(r, "info", tr("Checking for updates — reload in a moment to see the result."))
	} else {
		s.flash(r, "info", tr("This router is already checking for updates."))
	}
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

// updatesSection leads Maintenance with the two lanes. It reads only the cache, so
// it costs a page render nothing; a router that has never checked says so and
// offers the check, which is the whole content of that state.
func updatesSection() widget.Widget {
	truth, known := updateChecks.state()
	checking := updateChecks.running()
	children := []widget.Widget{}
	if err := updateChecks.takeFailure(); err != nil {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Update check failed",
			Body: fmt.Sprintf("This router could not read what it can install (%v).", err)})
	}
	if err := packageUpgrade.takeFailure(); err != nil {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Update did not complete",
			Body: fmt.Sprintf("Installing the available packages failed (%v). Nothing was left half-installed — apk applies an update as one transaction.", err)})
	}
	children = append(children, softwareLane(truth, known, checking), firmwareLane(truth, known))
	return &widget.Section{
		Title:    "Updates",
		Sub:      updatesCheckedLine(truth, known, checking),
		Control:  updatesCheckForm(checking),
		Children: children,
	}
}

// updatesCheckedLine is the quiet freshness statement under the heading — the age
// of the answer every lane below is reading.
func updatesCheckedLine(truth updateTruth, known, checking bool) string {
	switch {
	case checking:
		return "Asking the package feeds and the update server what this router could install…"
	case !known:
		return "This router has not looked for updates yet. Nothing is installed without your say-so."
	default:
		return "Checked **" + humanAgo(time.Since(truth.CheckedAt)) + "**. Nothing is installed without your say-so."
	}
}

// updatesCheckForm is the section's one control: the check itself, which reaches
// the network and so can only be a background run.
func updatesCheckForm(checking bool) widget.Widget {
	button := &widget.Button{Label: "Check for updates", Style: "secondary", Icon: "refresh-cw", Name: "action", Value: "check"}
	if checking {
		button.Label, button.Loading = "Checking…", true
	}
	return &widget.Form{Action: maintenancePath + "/updates/check", NoSubmit: true, Fields: []widget.Widget{button}}
}

// softwareLane is the package half: what the feeds hold newer copies of, Verso's
// own package leading, and the one act that installs them all.
func softwareLane(truth updateTruth, known, checking bool) widget.Widget {
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
	section.Children = []widget.Widget{
		upgradableList(truth.Packages),
		&widget.Form{Action: maintenancePath + "/updates/install", NoSubmit: true, Fields: []widget.Widget{
			updatesInstallButton(checking),
		}},
	}
	return section
}

// softwareLead names the update a person came for. Verso's own package is that one
// whenever it is in the set; otherwise the set speaks as a count.
func softwareLead(truth updateTruth) string {
	verso, ok := truth.verso()
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

// upgradableList names every package the update would change: Verso's own first,
// the rest folded away, because a person acts on the lead and audits the tail.
func upgradableList(packages []openwrt.PackageUpgrade) widget.Widget {
	rows := make([]widget.SettingsItem, 0, len(packages))
	var lead []widget.SettingsItem
	for _, p := range packages {
		row := widget.SettingsItem{
			Title: p.Name,
			Desc:  "This router runs " + p.Installed,
			Value: p.Available,
		}
		if p.Name == versoPackage {
			lead = append(lead, row)
			continue
		}
		rows = append(rows, row)
	}
	card := &widget.Settings{Style: "card", Items: lead}
	if len(card.Items) == 0 {
		card.Items, rows = rows[:1], rows[1:]
	}
	if len(rows) != 0 {
		card.Seam = &widget.SettingsSeam{Summary: seamSummary(len(rows)), Items: rows}
	}
	return card
}

func seamSummary(n int) string {
	if n == 1 {
		return "1 more package"
	}
	return strconv.Itoa(n) + " more packages"
}

func updatesInstallButton(checking bool) *widget.Button {
	button := &widget.Button{Label: "Update now", Style: "primary", Icon: "download", Name: "action", Value: "install"}
	switch {
	case packageUpgrade.running():
		button.Label, button.Loading = "Installing…", true
	case feedRefresh.running() || checking:
		button.Disabled = true
	}
	return button
}

// firmwareLane is the system half. Where the package lane always has an answer,
// this one may only have a reason there is none — and a named reason is what the
// person needs, so each rung says its own plain sentence.
func firmwareLane(truth updateTruth, known bool) widget.Widget {
	section := &widget.Section{Title: "System firmware", Hairline: true}
	if !known {
		section.Sub = "Whether a newer OpenWrt build exists for this router is not known until it checks."
		return section
	}
	firmware := truth.Firmware
	switch firmware.State {
	case openwrt.FirmwareUpdateAvailable:
		section.Sub = fmt.Sprintf("**OpenWrt %s is available for this router.** It runs %s.", firmware.To, firmware.From)
		section.Children = []widget.Widget{firmwareFacts(firmware)}
	case openwrt.FirmwareCurrent:
		// The build it runs is right below in the facts, so the sentence names no
		// version — one fact, one place, and a sentence a catalog can translate.
		section.Sub = "This router runs the newest build its update server offers."
		section.Children = []widget.Widget{firmwareFacts(firmware)}
	case openwrt.FirmwareNoOwut:
		section.Sub = "This router cannot check for firmware builds: the upgrade tool it needs, **owut**, is not installed."
		section.Children = []widget.Widget{
			&widget.Link{Style: "button", Label: "Install owut", Icon: "download", Href: packagesPath + "/discover?q=owut"},
		}
	case openwrt.FirmwareNoServer:
		section.Sub = "No update server answered, so this router could not find out whether a newer build exists."
		section.Children = []widget.Widget{firmwareComplaint(firmware)}
	case openwrt.FirmwareUnsupported:
		section.Sub = "The update server cannot build an image for this router, so there is no firmware update to offer."
		section.Children = []widget.Widget{firmwareComplaint(firmware)}
	default:
		section.Sub = "The firmware check could not run on this router."
	}
	return section
}

// firmwareFacts is the check's own working: which server was asked, and how much a
// build would change. It is what makes the sentence above it verifiable.
func firmwareFacts(firmware openwrt.FirmwareUpdate) widget.Widget {
	items := make([]widget.Property, 0, 4)
	if firmware.From != "" {
		items = append(items, widget.Property{Label: "Installed build", Value: firmware.From, Mono: true, Emphasis: true})
	}
	if firmware.To != "" && firmware.To != firmware.From {
		items = append(items, widget.Property{Label: "Available build", Value: firmware.To, Mono: true, Emphasis: true})
	}
	if firmware.Server != "" {
		items = append(items, widget.Property{Label: "Update server", Value: firmware.Server, Mono: true})
	}
	if firmware.Packages > 0 {
		items = append(items, widget.Property{Label: "Packages it would change", Value: strconv.Itoa(firmware.Packages)})
	}
	return &widget.Properties{Style: "system", Items: items}
}

// firmwareComplaint repeats the tool's own words, so a person can take the exact
// message to a forum or a support request rather than a paraphrase of it.
func firmwareComplaint(firmware openwrt.FirmwareUpdate) widget.Widget {
	body := firmware.Message
	if body == "" {
		body = "The check gave no reason."
	}
	if firmware.Server != "" {
		body = firmware.Server + " — " + body
	}
	return &widget.Callout{Variant: "neutral", Compact: true, Body: body}
}

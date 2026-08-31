// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

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

// updateChecks is the one check allowed to run at a time, and packageUpgrade the
// installation it leads to. Both are device-wide: what a router can install is a
// property of the router, not of whoever is looking at it. Neither holds the
// answer — only whether a run is under way, and how the last one failed.
var (
	updateChecks   backgroundJob
	packageUpgrade backgroundJob
)

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

// updatesSection leads Maintenance with the two lanes. It reads only the recorded
// answer, so it costs a page render nothing; a router that has never checked says
// so and offers the check, which is the whole content of that state.
func (s *Server) updatesSection(ctx context.Context, sid string) widget.Widget {
	truth, known := s.updateTruth()
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
	children = append(children, softwareLane(truth, known, checking), firmwareLane(truth, known), s.autocheckLane(ctx, sid))
	return &widget.Section{
		Title:    "Updates",
		Sub:      updatesCheckedLine(truth, known, checking),
		Control:  updatesCheckForm(checking),
		Children: children,
	}
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
	return &widget.Form{
		Action: maintenancePath + "/updates/autocheck",
		Submit: "Save",
		Fields: []widget.Widget{&widget.Settings{Items: []widget.SettingsItem{{
			Title: "Check for updates automatically",
			Desc:  "Once a day this router asks what is available. It installs nothing on its own.",
			Code:  updatesSectionName + "." + autocheckOption,
			Toggle: &widget.SettingsToggle{
				Name: autocheckOption,
				On:   value == optionOn,
			},
		}}}},
	}
}

// handleUpdatesAutocheck stages the daily check's gate. A switch that is off
// posts nothing, so an absent field is the person saying no — and that no is
// written as an explicit 0 rather than by clearing the option, because an absence
// is what the next package install seeds back to on (ADR-014 §2).
func (s *Server) handleUpdatesAutocheck(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	// The switch posts its value only when on, and that value is optionOn — so
	// exactly that string means yes, and anything else (a different value, an
	// absent field) means no. This mirrors the reader's rule: optionOn is the only
	// truth for "on" (see settings.go).
	value := optionOff
	if r.PostForm.Get(autocheckOption) == optionOn {
		value = optionOn
	}
	if err := s.stageVersoOption(r.Context(), s.sessionSID(r), updatesSectionName, autocheckOption, value); err != nil {
		log.Printf("verso: updates: staging the automatic check failed: %v", err)
		s.flash(r, "danger", tr("That setting could not be saved just now. Try again in a moment."))
	} else {
		s.flash(r, "info", tr("Saved. Use Save & Apply to put the change into effect."))
	}
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}

// updatesCheckedLine is the quiet freshness statement under the heading — the age
// of the answer every lane below is reading.
func updatesCheckedLine(truth updatecheck.Truth, known, checking bool) string {
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
	section.Children = []widget.Widget{
		upgradableList(truth),
		&widget.Form{Action: maintenancePath + "/updates/install", NoSubmit: true, Fields: []widget.Widget{
			updatesInstallButton(checking),
		}},
	}
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

// upgradableList names every package the update would change: Verso's own as
// the lead when it is among them, and the full manifest folded behind a
// disclosure as a condensed table — a person acts on the lead and audits the
// tail, and the tail is tabular fact (name, version → version), not a run of
// options.
func upgradableList(truth updatecheck.Truth) widget.Widget {
	var children []widget.Widget
	if verso, ok := truth.Verso(); ok {
		children = append(children, &widget.Settings{Style: "card", Items: []widget.SettingsItem{{
			Title: verso.Name,
			Desc:  "This router runs " + verso.Installed,
			Value: verso.Available,
		}}})
		// A lead that is the whole story needs no manifest behind it.
		if len(truth.Packages) == 1 {
			return children[0]
		}
	}
	rows := make([]widget.TableRow, 0, len(truth.Packages))
	for _, p := range truth.Packages {
		rows = append(rows, widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
			{Text: p.Name},
			{Text: p.Installed + " → " + p.Available},
		}})
	}
	children = append(children, &widget.Disclosure{
		Summary: changesSummary(len(truth.Packages)),
		Children: []widget.Widget{&widget.Table{
			Condensed: true,
			Columns: []widget.TableColumn{
				{Label: "Package", Kind: "name"},
				{Label: "Version", Kind: "mono"},
			},
			Rows: rows,
		}},
	})
	return &widget.Stack{Children: children}
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
	case feedRefresh.running() || checking:
		button.Disabled = true
	}
	return button
}

// firmwareLane is the system half. Where the package lane always has an answer,
// this one may only have a reason there is none — and a named reason is what the
// person needs, so each rung says its own plain sentence.
func firmwareLane(truth updatecheck.Truth, known bool) widget.Widget {
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

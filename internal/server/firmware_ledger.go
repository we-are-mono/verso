// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// The firmware ledger is Maintenance's firmware section: a verdict under its
// state mark, then a small table of what this router runs, part by part, with an
// Available column beside it whenever the update server offers a newer build.
// The answer to "is there an update" is a diff, so the section draws one.

// ledgerChange is what the Available column can honestly say about one part.
type ledgerChange string

const (
	ledgerChanged    ledgerChange = "changed"    // the server named a new value
	ledgerSame       ledgerChange = "same"       // a sysupgrade cannot change it
	ledgerUnreported ledgerChange = "unreported" // the server did not say
)

type ledgerRow struct {
	Label, Current, CurrentRev, Next, NextRev string
	Change                                    ledgerChange // empty while nothing is offered
}

// firmwareLedger is the section's model. Every string is already localized.
type firmwareLedger struct {
	Mark      string // "green" | "denim" | "marigold" | "hollow"
	Title     string
	Lede      string
	Complaint string // the update tool's own words, verbatim
	Rows      []ledgerRow
	Offer     bool          // a newer build is on offer: the Available column shows
	Server    template.HTML // which server answered, with the name in mono
	Changes   string        // how many packages the offered build changes
	NeedsOwut bool          // the check cannot run until owut is installed
}

// firmwareLedgerView reads the recorded check and the running board into the
// ledger. The check names the build on offer and nothing else about it, so the
// Available column fills the OpenWrt row alone: the target cannot change under a
// sysupgrade, and the kernel and Verso the server did not report. A check under
// way is not a verdict: the Check again button carries it, and the verdict keeps
// saying what the last check found.
func firmwareLedgerView(tr func(string) string, truth updatecheck.Truth, known bool, board openwrt.Board, verso string) firmwareLedger {
	firmware := truth.Firmware
	running := board.Firmware
	if running == "" {
		running = firmware.From
	}
	kernel := strings.TrimPrefix(board.Kernel, "Linux ")
	if kernel == "" {
		kernel, _, _ = strings.Cut(board.KernelBuild, " ")
	}
	version, rev := splitBuild(running)
	l := firmwareLedger{Rows: []ledgerRow{
		{Label: "OpenWrt", Current: version, CurrentRev: rev},
		{Label: tr("Kernel"), Current: kernel},
		{Label: "Verso", Current: verso},
		{Label: tr("Target"), Current: board.Target},
	}}

	if !known {
		l.Mark, l.Title = "hollow", tr("Not checked yet")
		l.Lede = tr("Whether a newer OpenWrt build exists for this router is not known until it checks.")
		return l
	}

	switch firmware.State {
	case openwrt.FirmwareUpdateAvailable:
		next, nextRev := splitBuild(firmware.To)
		l.Mark, l.Title = "denim", fmt.Sprintf(tr("%s is available"), next)
		l.Lede = tr("The update server is offering a newer build for this router.")
		l.Offer = true
		l.Rows[0].Next, l.Rows[0].NextRev, l.Rows[0].Change = next, nextRev, ledgerChanged
		l.Rows[1].Change, l.Rows[2].Change, l.Rows[3].Change = ledgerUnreported, ledgerUnreported, ledgerSame
		if firmware.Server != "" {
			l.Server = verbatimIn(tr("Built by %s"), firmware.Server)
		}
		if firmware.Packages > 0 {
			l.Changes = counted(tr, int64(firmware.Packages), "1 package changes", "%d packages change")
		}
	case openwrt.FirmwareCurrent:
		l.Mark, l.Title = "green", tr("Up to date")
		l.Lede = tr("This router runs the newest build its update server offers.")
		if firmware.Server != "" {
			l.Server = verbatimIn(tr("Checked against %s"), firmware.Server)
		}
	case openwrt.FirmwareNoOwut:
		l.Mark, l.Title = "marigold", tr("Firmware checks need owut")
		l.Lede = tr("The upgrade tool this router needs to check for firmware builds, owut, is not installed.")
		l.NeedsOwut = true
	case openwrt.FirmwareNoServer:
		l.Mark, l.Title = "marigold", tr("The update server didn't answer")
		l.Lede = tr("No update server answered, so this router could not find out whether a newer build exists.")
		l.Complaint = firmwareComplaint(firmware)
	case openwrt.FirmwareUnsupported:
		l.Mark, l.Title = "marigold", tr("No firmware updates for this router")
		l.Lede = tr("The update server cannot build an image for this router, so there is no firmware update to offer.")
		l.Complaint = firmwareComplaint(firmware)
	default:
		l.Mark, l.Title = "marigold", tr("The firmware check could not run")
		l.Lede = tr("Check again in a moment, or upload an image yourself.")
	}
	return l
}

// splitBuild parts an OpenWrt build string into its version and its revision:
// "OpenWrt 25.12.4 r32933-4ccb782af7" → "25.12.4", "r32933-4ccb782af7". The
// distribution name is dropped; the ledger's row already says OpenWrt.
func splitBuild(build string) (version, revision string) {
	build = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(build), "OpenWrt "))
	version, revision, _ = strings.Cut(build, " ")
	return version, strings.TrimSpace(revision)
}

// firmwareComplaint is the update tool's own words under the plain sentence:
// which server was asked and what came back, verbatim, for whoever takes it to a
// forum. A check that said nothing leaves it empty.
func firmwareComplaint(firmware openwrt.FirmwareUpdate) string {
	parts := make([]string, 0, 2)
	if firmware.Server != "" {
		parts = append(parts, firmware.Server)
	}
	if firmware.Message != "" {
		parts = append(parts, firmware.Message)
	}
	return strings.Join(parts, " — ")
}

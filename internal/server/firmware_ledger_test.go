// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"reflect"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// TestFirmwareLedgerStatesOnlyWhatTheServerReported: the update check names the
// build on offer and nothing else about it, so the Available column fills the
// OpenWrt row alone. The target cannot change under a sysupgrade, so it reads
// "same"; the kernel and Verso the server did not report, so they say so rather
// than borrow a guess.
func TestFirmwareLedgerStatesOnlyWhatTheServerReported(t *testing.T) {
	board := openwrt.Board{Firmware: "OpenWrt 25.12.4 r32933-4ccb782af7", KernelBuild: "6.12.101 #1 SMP PREEMPT_DYNAMIC", Target: "layerscape/armv8_64b"}
	truth := updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{
		State: openwrt.FirmwareUpdateAvailable, From: "25.12.4 r32933", To: "25.12.5 r33051", Server: "https://sysupgrade.openwrt.org", Packages: 1,
	}}
	l := firmwareLedgerView(identity, truth, true, board, "0.0.36")

	want := []ledgerRow{
		{Label: "OpenWrt", Current: "25.12.4", CurrentRev: "r32933-4ccb782af7", Next: "25.12.5", NextRev: "r33051", Change: ledgerChanged},
		{Label: "Kernel", Current: "6.12.101", Change: ledgerUnreported},
		{Label: "Verso", Current: "0.0.36", Change: ledgerUnreported},
		{Label: "Target", Current: "layerscape/armv8_64b", Change: ledgerSame},
	}
	if !reflect.DeepEqual(l.Rows, want) {
		t.Errorf("ledger rows:\n got %+v\nwant %+v", l.Rows, want)
	}
	if !l.Offer || l.Title != "" {
		t.Errorf("an offered build opens the Available column and explains nothing: offer=%v title=%q", l.Offer, l.Title)
	}
	if l.Changes != "1 package changes" {
		t.Errorf("changes = %q, want the singular", l.Changes)
	}
}

// TestFirmwareLedgerExplainsOnlyAWarning: the section explains itself only when
// something is wrong — the marigold band, with its title and sentence. A router
// not yet checked, one up to date, and one with a build on offer say nothing in
// words: the heading line says when it last checked, and the ledger's Available
// column says what is on offer.
func TestFirmwareLedgerExplainsOnlyAWarning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		truth updatecheck.Truth
		known bool
		title string // empty: no explanation
	}{
		{"never checked", updatecheck.Truth{}, false, ""},
		{"current", updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent}}, true, ""},
		{"no owut", updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoOwut}}, true, "Firmware checks need owut"},
		{"no server", updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareNoServer}}, true, "The update server didn't answer"},
		{"unsupported", updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUnsupported}}, true, "No firmware updates for this router"},
		{"could not run", updatecheck.Truth{}, true, "The firmware check could not run"},
	} {
		l := firmwareLedgerView(identity, tc.truth, tc.known, openwrt.Board{}, "0.0.36")
		if l.Title != tc.title || (tc.title == "") != (l.Lede == "") || l.Offer {
			t.Errorf("%s: title=%q lede=%q offer=%v; want title %q and no offer", tc.name, l.Title, l.Lede, l.Offer, tc.title)
		}
	}
}

// TestCheckedSaysWhen: beside Check again, the section says when it last
// checked — the clock alone the same day, the date with it otherwise — or that
// it has not checked yet.
func TestCheckedSaysWhen(t *testing.T) {
	now := time.Date(2026, 9, 27, 18, 5, 0, 0, time.Local)
	for _, tc := range []struct {
		name  string
		at    time.Time
		known bool
		want  string
	}{
		{"never", time.Time{}, false, "Not checked yet"},
		{"today", time.Date(2026, 9, 27, 14, 32, 0, 0, time.Local), true, "Checked at 14:32"},
		{"earlier", time.Date(2026, 9, 26, 9, 7, 0, 0, time.Local), true, "Checked 26 Sep, 09:07"},
	} {
		if got := checkedAt(identity, tc.at, tc.known, now); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestMaintenanceUptimeSingulars: the "1 …" forms carry no verb, so they must
// not be formatted with the count, or the page prints "%!(EXTRA …)".
func TestMaintenanceUptimeSingulars(t *testing.T) {
	for seconds, want := range map[int64]string{
		86400 + 3600:   "1 day, 1 hour",
		60:             "1 minute",
		2*86400 + 7200: "2 days, 2 hours",
	} {
		if got := maintenanceUptime(identity, seconds); got != want {
			t.Errorf("maintenanceUptime(%d) = %q, want %q", seconds, got, want)
		}
	}
}

// TestFirmwareLedgerKernelIsTheRelease: the row already says Kernel, so a board
// that reports "Linux 6.12.101" shows the release alone.
func TestFirmwareLedgerKernelIsTheRelease(t *testing.T) {
	for _, board := range []openwrt.Board{
		{Kernel: "Linux 6.12.101+deb13-amd64"},
		{Kernel: "6.12.101+deb13-amd64"},
		{KernelBuild: "6.12.101+deb13-amd64 #1 SMP"},
	} {
		if got := firmwareLedgerView(identity, updatecheck.Truth{}, false, board, "").Rows[1].Current; got != "6.12.101+deb13-amd64" {
			t.Errorf("kernel row for %+v = %q", board, got)
		}
	}
}

func TestSplitBuild(t *testing.T) {
	for in, want := range map[string][2]string{
		"OpenWrt 25.12.4 r32933-4ccb782af7": {"25.12.4", "r32933-4ccb782af7"},
		"25.12.5 r33051":                    {"25.12.5", "r33051"},
		"SNAPSHOT":                          {"SNAPSHOT", ""},
		"":                                  {"", ""},
	} {
		if v, rev := splitBuild(in); v != want[0] || rev != want[1] {
			t.Errorf("splitBuild(%q) = %q, %q; want %q, %q", in, v, rev, want[0], want[1])
		}
	}
}

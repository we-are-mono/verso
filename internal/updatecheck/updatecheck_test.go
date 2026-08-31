// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package updatecheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// fakeSource is the two read verbs and nothing else — the whole surface a check
// reads, so a test needs no device and no backend.
type fakeSource struct {
	packages    []openwrt.PackageUpgrade
	packagesErr error
	firmware    openwrt.FirmwareUpdate
	firmwareErr error
}

func (f fakeSource) PkgUpgradable(context.Context, string) ([]openwrt.PackageUpgrade, error) {
	return f.packages, f.packagesErr
}

func (f fakeSource) FirmwareCheck(context.Context, string) (openwrt.FirmwareUpdate, error) {
	return f.firmware, f.firmwareErr
}

// TestRunReadsBothLanes: one check is both verbs and the moment it finished.
func TestRunReadsBothLanes(t *testing.T) {
	source := fakeSource{
		packages: []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
		firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent, From: "25.12.4 r32933"},
	}
	truth, err := Run(context.Background(), source, openwrt.ZeroSID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(truth.Packages) != 1 || truth.Packages[0].Name != "verso" {
		t.Errorf("package lane = %+v, want the source's one upgrade", truth.Packages)
	}
	if truth.Firmware.State != openwrt.FirmwareCurrent {
		t.Errorf("firmware lane = %q, want the source's answer", truth.Firmware.State)
	}
	if truth.CheckedAt.IsZero() {
		t.Error("an answer with no timestamp cannot be shown as fresh or stale")
	}
}

// TestRunFailsOnThePackageLane: apk answers on every device, so its failure is
// the check's failure.
func TestRunFailsOnThePackageLane(t *testing.T) {
	_, err := Run(context.Background(), fakeSource{packagesErr: errors.New("helper unreachable")}, "sid")
	if err == nil {
		t.Fatal("a package-lane failure should fail the check")
	}
}

// TestRunToleratesTheFirmwareLane: the firmware check answers for devices it can
// and reports a rung for the rest, so a transport failure leaves the lane empty
// rather than losing the package answer beside it.
func TestRunToleratesTheFirmwareLane(t *testing.T) {
	truth, err := Run(context.Background(), fakeSource{
		packages:    []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}},
		firmwareErr: errors.New("owut unreachable"),
	}, "sid")
	if err != nil {
		t.Fatalf("a firmware-lane failure should not fail the check: %v", err)
	}
	if len(truth.Packages) != 1 {
		t.Errorf("the package answer should survive: %+v", truth.Packages)
	}
	if truth.Firmware != (openwrt.FirmwareUpdate{}) {
		t.Errorf("an unanswered firmware lane should stay empty: %+v", truth.Firmware)
	}
}

// TestWriteThenRead: whoever ran the check, the next reader gets the same answer
// back — that is the whole contract between the cron run and the shell.
func TestWriteThenRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	want := Truth{
		Packages:  []openwrt.PackageUpgrade{{Name: "verso", Installed: "0.0.22", Available: "0.0.23"}},
		Firmware:  openwrt.FirmwareUpdate{State: openwrt.FirmwareUnsupported, Message: "File system type '(null)'"},
		CheckedAt: time.Now().Add(-90 * time.Minute).Round(time.Second),
	}
	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, known := Read(dir)
	if !known {
		t.Fatal("a written answer should read back")
	}
	if len(got.Packages) != 1 || got.Packages[0].Name != "verso" {
		t.Errorf("packages = %+v, want the written set", got.Packages)
	}
	if got.Firmware.State != openwrt.FirmwareUnsupported || got.Firmware.Message != want.Firmware.Message {
		t.Errorf("firmware = %+v, want the written rung and its message", got.Firmware)
	}
	if !got.CheckedAt.Equal(want.CheckedAt) {
		t.Errorf("checked_at = %v, want %v", got.CheckedAt, want.CheckedAt)
	}
}

// TestWriteCreatesAReadableFile: the two writers run under different accounts, so
// the file one leaves must be one the other can open.
func TestWriteCreatesAReadableFile(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Truth{CheckedAt: time.Now()}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, stateFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("state file mode = %v, want 0644 so either writer's file is readable", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the state dir holds %d entries, want only the state file", len(entries))
	}
}

// TestWriteReplacesTheAnswerInPlace: a second check overwrites the first, and a
// reader never sees a half-written file.
func TestWriteReplacesTheAnswerInPlace(t *testing.T) {
	dir := t.TempDir()
	first := time.Now().Add(-time.Hour).Round(time.Second)
	second := time.Now().Round(time.Second)
	if err := Write(dir, Truth{CheckedAt: first}); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(dir, Truth{CheckedAt: second}); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, known := Read(dir)
	if !known || !got.CheckedAt.Equal(second) {
		t.Errorf("checked_at = %v (known=%v), want the newer answer %v", got.CheckedAt, known, second)
	}
}

// TestReadWithoutAnAnswer: a router that has not checked — or has just booted
// into an empty tmpfs — says so rather than inventing a truth.
func TestReadWithoutAnAnswer(t *testing.T) {
	if _, known := Read(filepath.Join(t.TempDir(), "absent")); known {
		t.Error("an absent state dir should read as never checked")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFile), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, known := Read(dir); known {
		t.Error("an unreadable answer should read as never checked")
	}
	// A file that parses but carries no moment records no check that happened —
	// the empty and null JSON shapes a mistaken writer might leave.
	for _, body := range []string{"{}", "null", `{"packages":[]}`} {
		empty := t.TempDir()
		if err := os.WriteFile(filepath.Join(empty, stateFile), []byte(body), 0o644); err != nil {
			t.Fatalf("seed %q: %v", body, err)
		}
		if _, known := Read(empty); known {
			t.Errorf("a state file of %q should read as never checked, not as up to date", body)
		}
	}
}

// TestWriteRefusesASymlinkedStateDir: the state directory's parent is
// group-writable and sticky, so a plugin account could plant a symlink where the
// directory belongs. Write must refuse it rather than follow it into a directory
// this process does not own (a plugin-to-root escalation otherwise).
func TestWriteRefusesASymlinkedStateDir(t *testing.T) {
	base := t.TempDir()
	victim := filepath.Join(base, "victim")
	if err := os.Mkdir(victim, 0o755); err != nil {
		t.Fatalf("victim: %v", err)
	}
	link := filepath.Join(base, "state")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := Write(link, Truth{CheckedAt: time.Now()}); err == nil {
		t.Fatal("Write through a symlinked state dir should be refused")
	}
	if _, err := os.Stat(filepath.Join(victim, stateFile)); err == nil {
		t.Error("the refusal must not have written into the symlink's target")
	}
}

// TestPendingIsWhatCanBeInstalled: the one boolean the tile and the sidebar mark
// with, true for either lane and for nothing else.
func TestPendingIsWhatCanBeInstalled(t *testing.T) {
	if (Truth{}).Pending() {
		t.Error("an empty answer has nothing to install")
	}
	if !(Truth{Packages: []openwrt.PackageUpgrade{{Name: "dnsmasq"}}}).Pending() {
		t.Error("an upgradable package is something to install")
	}
	firmware := Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable}}
	if !firmware.Pending() {
		t.Error("an available build is something to install")
	}
	current := Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareCurrent}}
	if current.Pending() {
		t.Error("a current build is not something to install")
	}
}

// TestVersoNamesItsOwnUpdate: Verso's own package is the one a person came to
// read about, so it is findable by name in the set.
func TestVersoNamesItsOwnUpdate(t *testing.T) {
	truth := Truth{Packages: []openwrt.PackageUpgrade{
		{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"},
		{Name: "verso", Installed: "0.0.22", Available: "0.0.23"},
	}}
	verso, ok := truth.Verso()
	if !ok || verso.Available != "0.0.23" {
		t.Errorf("Verso() = %+v, %v; want Verso's own entry", verso, ok)
	}
	if _, ok := (Truth{Packages: []openwrt.PackageUpgrade{{Name: "dnsmasq"}}}).Verso(); ok {
		t.Error("a set without Verso should not claim one")
	}
}

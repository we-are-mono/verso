// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package updatecheck is the one update check this router runs and the one file
// it leaves behind. Two callers reach it — the daily cron run as local root, and
// the shell's Check-for-updates button as the operator — and ADR-014 §5 has them
// converge here: one two-lane read, one format, one freshness stamp, whoever
// produced it. Every surface that reports what the router could install reads
// that file and nothing else, so a shell restart or a reboot costs the answer's
// accuracy nothing but its age.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// DefaultDir is where the answer lives: a subdirectory of Verso's runtime dir,
// owned by the verso account. The parent is sticky and root-owned, which stops a
// group member renaming over another account's file — including the shell's own
// rename over a root-written answer. Its own directory, owned by the shell's
// account, lets both writers replace the file atomically while the sticky parent
// still keeps any other group member from swapping the directory itself.
// Runtime state, not settings: it is tmpfs, and a reboot clearing it is honest
// (ADR-013 §4).
const DefaultDir = "/var/run/verso/state"

// stateFile is the answer's name inside that directory.
const stateFile = "updates.json"

// stateOwner is the uid and gid of the de-privileged `verso` account the shell
// runs as, mirroring VERSO_UID in verso-rpcd. A root-run check creates the
// directory on the shell's behalf, so the shell can write into it afterwards.
const stateOwner = 6000

// versoPackage is Verso's own package name. It leads the software lane, because
// an update to the thing the person is looking at is the one they came to read
// about.
const versoPackage = "verso"

// Truth is one complete answer from both lanes, and the moment it was read. It
// is what crosses the file: pre-alpha, both writers ship together, so the shape
// carries no version of its own.
type Truth struct {
	Packages  []openwrt.PackageUpgrade `json:"packages"`
	Firmware  openwrt.FirmwareUpdate   `json:"firmware"`
	CheckedAt time.Time                `json:"checked_at"`
}

// Pending reports whether the device has anything to install — the one boolean
// the homepage tile and the sidebar's device row are allowed to draw attention
// with.
func (t Truth) Pending() bool {
	return len(t.Packages) != 0 || t.Firmware.State == openwrt.FirmwareUpdateAvailable
}

// Verso returns Verso's own entry in the upgradable set, if it is in it.
func (t Truth) Verso() (openwrt.PackageUpgrade, bool) {
	for _, p := range t.Packages {
		if p.Name == versoPackage {
			return p, true
		}
	}
	return openwrt.PackageUpgrade{}, false
}

// Source is the pair of read verbs a check consults: what the feeds hold newer
// copies of, and what the attended-sysupgrade server can build. Narrower than the
// whole backend on purpose — an unattended check must be unable to reach a write
// verb even by accident — and satisfied by openwrt.Backend as it stands.
type Source interface {
	PkgUpgradable(ctx context.Context, sid string) ([]openwrt.PackageUpgrade, error)
	FirmwareCheck(ctx context.Context, sid string) (openwrt.FirmwareUpdate, error)
}

// Run reads both lanes under the given session and stamps the moment.
//
// The package lane failing is the check failing — apk answers on every device.
// The firmware lane failing is not: the helper reports a rung rather than an
// error for every device it cannot answer for, so a transport failure here
// leaves the lane's state empty and the surfaces say the check could not run.
func Run(ctx context.Context, source Source, sid string) (Truth, error) {
	packages, err := source.PkgUpgradable(ctx, sid)
	if err != nil {
		return Truth{}, err
	}
	firmware, err := source.FirmwareCheck(ctx, sid)
	if err != nil {
		log.Printf("verso: updates: firmware check unavailable: %v", err)
		firmware = openwrt.FirmwareUpdate{}
	}
	return Truth{Packages: packages, Firmware: firmware, CheckedAt: time.Now()}, nil
}

// Write replaces the answer in dir atomically: a temporary file beside it, then
// a rename, so a reader never opens a half-written truth. The file is world
// readable because the two writers are different accounts and each must be able
// to read what the other left.
func Write(dir string, t Truth) error {
	if err := ensureDir(dir); err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	staged, err := os.CreateTemp(dir, "updates-*.json")
	if err != nil {
		return err
	}
	name := staged.Name()
	written := false
	defer func() {
		if !written {
			_ = os.Remove(name)
		}
	}()
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Chmod(0o644); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, stateFile)); err != nil {
		return err
	}
	written = true
	return nil
}

// ensureDir makes the state directory when nothing has yet — the first check
// after a boot that cleared tmpfs, or a device whose init script predates it.
// A root-run check hands ownership to the shell's account, so the operator's own
// check can replace the file afterwards; the shell, already running as that
// account, creates a directory it owns anyway.
//
// The state directory's parent is group-writable and sticky, so a plugin account
// can plant a symlink where this directory belongs. Lstat (never Stat) rejects
// anything that is not already a real directory, and the final component is made
// with Mkdir (never MkdirAll, which follows a planted link): Mkdir fails on an
// existing symlink rather than following it, so a chown can never land on a
// directory this process does not own.
func ensureDir(dir string) error {
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("updatecheck: state path %q is not a directory", dir)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The parent lives under root-owned /var/run, which no unprivileged account
	// can write, so creating it does not carry the final component's risk.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Chown(dir, stateOwner, stateOwner)
}

// Read returns the answer in dir and whether there is one at all. A router that
// has never checked, and one whose answer cannot be parsed, are the same to every
// surface: they say so plainly rather than inventing a truth.
func Read(dir string) (Truth, bool) {
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return Truth{}, false
	}
	var t Truth
	if err := json.Unmarshal(data, &t); err != nil {
		log.Printf("verso: updates: the recorded answer could not be read: %v", err)
		return Truth{}, false
	}
	// A file that parses but carries no moment — a bare `{}` or `null` — records
	// no check that happened. It reads as never-checked, not as a check that found
	// the router up to date in the first instant of 1970.
	if t.CheckedAt.IsZero() {
		return Truth{}, false
	}
	return t, true
}

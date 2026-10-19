// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// usageSetup points nlbwmon at the history Verso reads (ADR-018).
const usageSetup = "../../docker/rootfs/usr/libexec/verso/usage-setup"

// fakeUCI is uci over a file of key=value lines ($UCI_STATE), each set and
// commit also written to $CALLS.
const fakeUCI = `#!/bin/sh
[ "$1" = -q ] && shift
case $1 in
get)
	v=$(grep -F "$2=" "$UCI_STATE" | head -n 1)
	[ -n "$v" ] || exit 1
	echo "${v#*=}" ;;
set)
	k=${2%%=*}
	grep -vF "$k=" "$UCI_STATE" > "$UCI_STATE.next"
	echo "$2" >> "$UCI_STATE.next"
	mv "$UCI_STATE.next" "$UCI_STATE"
	echo "uci set $2" >> "$CALLS" ;;
commit)
	echo "uci commit $2" >> "$CALLS" ;;
esac
`

// fakeInit is nlbwmon's init script: running as $RUNNING says, and each other
// verb written to $CALLS.
const fakeInit = `#!/bin/sh
[ "$1" = running ] && { [ "$RUNNING" = yes ]; exit; }
echo "nlbwmon $1" >> "$CALLS"
`

const (
	cfg      = "nlbwmon.@nlbwmon[0]"
	stock    = cfg + "=nlbwmon\n" + cfg + ".database_interval=1\n" + cfg + ".database_generations=10\n" + cfg + ".database_directory=/var/lib/nlbwmon\n"
	versos   = cfg + "=nlbwmon\n" + cfg + ".database_interval=2026-01-01/1\n" + cfg + ".database_generations=400\n" + cfg + ".database_directory=/usr/share/nlbwmon/db\n"
	noConfig = ""
)

// runUsageSetup runs usage-setup in mode under busybox sh over a router whose
// nlbwmon settings are state, nlbwmon running or not, and says what it set,
// committed and asked of nlbwmon.
func runUsageSetup(t *testing.T, mode, state string, running bool) []string {
	t.Helper()
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("usage-setup runs under busybox sh, which is not installed")
	}
	dir := t.TempDir()
	write := func(name, body string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	write("bin/uci", fakeUCI, 0o755)
	initPath := write("nlbwmon", fakeInit, 0o755)
	statePath := write("state", state, 0o644)
	calls := filepath.Join(dir, "calls")
	runningWord := "no"
	if running {
		runningWord = "yes"
	}
	cmd := exec.Command(busybox, "sh", usageSetup, mode)
	cmd.Env = []string{
		"PATH=" + bin + ":" + filepath.Dir(busybox) + ":/usr/bin:/bin",
		"UCI_STATE=" + statePath, "CALLS=" + calls, "RUNNING=" + runningWord, "NLBWMON_INIT=" + initPath,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("usage-setup %s: %v\n%s", mode, err, out)
	}
	data, _ := os.ReadFile(calls)
	return strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(data)), " ", "_"))
}

// TestAnUpgradeLeavesNlbwmonRunning: settings already Verso's change nothing,
// so nlbwmon is neither committed nor restarted. A restart reads the whole
// connection table again at once: on a busy router it logs a line for every
// connection past its database limit, and counts those without their device.
func TestAnUpgradeLeavesNlbwmonRunning(t *testing.T) {
	if got := runUsageSetup(t, "install", versos, true); len(got) != 0 {
		t.Errorf("an upgrade with nothing to change did %v", got)
	}
}

// TestAFirstInstallTakesNlbwmonOver: stock settings are set to Verso's,
// committed, and a running nlbwmon restarted to take them up.
func TestAFirstInstallTakesNlbwmonOver(t *testing.T) {
	got := runUsageSetup(t, "install", stock, true)
	for _, want := range []string{
		"uci_set_" + cfg + ".database_interval=2026-01-01/1",
		"uci_set_" + cfg + ".database_generations=400",
		"uci_commit_nlbwmon",
		"nlbwmon_restart",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("first install did %v, missing %s", got, want)
		}
	}
	if got[len(got)-1] != "nlbwmon_restart" {
		t.Errorf("nlbwmon restarted before the settings were committed: %v", got)
	}
}

// TestAStoppedNlbwmonIsNotStarted: usage-setup restarts only a running
// nlbwmon; starting a stopped one is the package script's.
func TestAStoppedNlbwmonIsNotStarted(t *testing.T) {
	got := runUsageSetup(t, "install", stock, false)
	if slices.Contains(got, "nlbwmon_restart") || slices.Contains(got, "nlbwmon_start") || !slices.Contains(got, "uci_commit_nlbwmon") {
		t.Errorf("a stopped nlbwmon: %v", got)
	}
}

// TestRemovingGivesNlbwmonItsStockSettings: removal sets the stock period and
// generations back and restarts a running nlbwmon.
func TestRemovingGivesNlbwmonItsStockSettings(t *testing.T) {
	got := runUsageSetup(t, "remove", versos, true)
	for _, want := range []string{
		"uci_set_" + cfg + ".database_interval=1",
		"uci_set_" + cfg + ".database_generations=10",
		"uci_set_" + cfg + ".database_directory=/var/lib/nlbwmon",
		"uci_commit_nlbwmon",
		"nlbwmon_restart",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("removal did %v, missing %s", got, want)
		}
	}
	if again := runUsageSetup(t, "remove", stock, true); len(again) != 0 {
		t.Errorf("removal over stock settings did %v", again)
	}
}

// TestNoNlbwmonConfigIsLeftAlone.
func TestNoNlbwmonConfigIsLeftAlone(t *testing.T) {
	if got := runUsageSetup(t, "install", noConfig, true); len(got) != 0 {
		t.Errorf("no nlbwmon config: %v", got)
	}
}

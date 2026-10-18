// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runInit runs script, the init script's functions named and then what
// follows, under busybox sh, with the chosen directory cfg and HISTORY_FROM in
// a scratch directory. running stands in for the running shell's directory
// (history_running reads procd and /proc). It answers what the script printed
// and the record HISTORY_FROM holds after, "" for none.
func runInit(t *testing.T, cfg, running, record, script string, functions ...string) (string, string) {
	t.Helper()
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("the init script runs under busybox sh, which is not installed")
	}
	scratch := t.TempDir()
	from := filepath.Join(scratch, "history-from")
	if record != "" {
		if err := os.WriteFile(from, []byte(record+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var body strings.Builder
	body.WriteString(`HISTORY_FROM=$1 CFG=$2 RUNNING=$3
config_get() { eval "$1=\$CFG"; }
`)
	for _, name := range functions {
		body.WriteString(initFunction(t, initScript, name))
	}
	body.WriteString("history_running() { echo \"$RUNNING\"; }\n")
	body.WriteString(script)
	cmd := exec.Command(busybox, "sh", "-c", body.String(), "sh", from, cfg, running)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v: %s", script, err, stderr.String())
	}
	got, err := os.ReadFile(from)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(out), "\n"), strings.TrimSuffix(string(got), "\n")
}

// TestAStartMovesTheHistoryFromWhereTheShellKeptIt: a directory chosen while
// the shell runs reaches it as a SIGHUP, so it writes on where it was started.
// The apply's start hands the shell, for its respawns, the directory it keeps
// the history in, and records it for the start after a stop, a shutdown or a
// power cut, which takes the record once. A shell kept where it is chosen
// moves nothing and leaves no record (ADR-019).
func TestAStartMovesTheHistoryFromWhereTheShellKeptIt(t *testing.T) {
	functions := []string{"history_choice", "history_note", "history_from"}
	cases := []struct {
		name, cfg, running, record string
		script                     string
		want, wantRecord           string
	}{
		{name: "an apply that chose another", cfg: "/mnt/sda1/verso", running: "/data/verso",
			script: "history_from\n", want: "/data/verso", wantRecord: "/data/verso"},
		{name: "an apply that chose it back", cfg: "/data/verso", running: "/data/verso", record: "/data/verso",
			script: "history_from\n", want: "/data/verso"},
		{name: "a start after the record", cfg: "/mnt/sda1/verso", record: "/data/verso",
			script: "history_from\n", want: "/data/verso"},
		{name: "a start with nothing chosen since", cfg: "/data/verso",
			script: "history_from\n"},
		{name: "a stop with another chosen", cfg: "/mnt/sda1/verso", running: "/data/verso",
			script: "history_note >/dev/null\n", wantRecord: "/data/verso"},
		{name: "a stop where it is chosen", cfg: "/data/verso", running: "/data/verso", record: "/tmp/old",
			script: "history_note >/dev/null\n"},
		{name: "a stop with no shell running", cfg: "/mnt/sda1/verso", record: "/data/verso",
			script: "history_note >/dev/null\n", wantRecord: "/data/verso"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, record := runInit(t, c.cfg, c.running, c.record, c.script, functions...)
			if got != c.want || record != c.wantRecord {
				t.Errorf("printed %q with the record %q, want %q with %q", got, record, c.want, c.wantRecord)
			}
		})
	}
}

// TestTheRunningShellsDirectoryIsReadFromItsArguments: history_flag reads the
// shell's arguments as start_service hands them over, after ujail's own when
// procd runs the shell in its jail.
func TestTheRunningShellsDirectoryIsReadFromItsArguments(t *testing.T) {
	for args, want := range map[string]string{
		"/usr/bin/verso\n-listen-https\n0.0.0.0:443\n-history-dir\n/data/verso\n-probe\n1.1.1.1\n": "/data/verso",
		"/sbin/ujail\n-t\n5\n-n\nverso\n--\n/usr/bin/verso\n-history-dir\n/mnt/sda1/my history\n":  "/mnt/sda1/my history",
		"/usr/bin/verso\n-listen-https\n0.0.0.0:443\n":                                             "",
		"/sbin/other\n-history-dir\n/data/verso\n":                                                 "",
	} {
		script := "printf '%s' \"$ARGS\" | history_flag\n"
		t.Setenv("ARGS", args)
		got, _ := runInit(t, "", "", "", script, "history_flag")
		if got != want {
			t.Errorf("history_flag on %q = %q, want %q", args, got, want)
		}
	}
}

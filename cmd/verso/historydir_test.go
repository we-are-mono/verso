// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// initScript is the script procd starts the shell through.
const initScript = "../../docker/rootfs/etc/init.d/verso"

// historyDirHarness runs the init script's history_dir against a router
// rooted at $ROOT: the paths it reads, makes and tests are taken below it,
// config_get answers the configured directory, /proc/mounts is $MOUNTS, and
// each chown is written to $CHOWNS rather than made.
const historyDirHarness = `ROOT=$1 CFG=$2 MOUNTS=$3 CHOWNS=$4 BB=$5
config_get() { eval "$1=\$CFG"; }
grep() { [ "$3" = /proc/mounts ] && set -- "$1" "$2" "$MOUNTS"; "$BB" grep "$@"; }
readlink() { r=$("$BB" readlink -f "$ROOT$2") || return 1; echo "${r#"$ROOT"}"; }
mkdir() { "$BB" mkdir "$1" "$2" "$ROOT$3"; }
chown() { echo "$*" >>"$CHOWNS"; }
[() {
	case $1 in
		-d | -L) set -- "$1" "$ROOT$2" "$3" ;;
		!) case $2 in -d | -L) set -- "$1" "$2" "$ROOT$3" "$4" ;; esac ;;
	esac
	command [ "$@"
}
`

// historyDirFunction is history_dir as the init script spells it.
func historyDirFunction(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile(initScript)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(script, []byte("\nhistory_dir() {\n"))
	if start < 0 {
		t.Fatalf("%s holds no history_dir", initScript)
	}
	end := bytes.Index(script[start:], []byte("\n}\n"))
	if end < 0 {
		t.Fatalf("%s: history_dir does not end", initScript)
	}
	return string(script[start+1 : start+end+3])
}

// router is a router's filesystem below a test directory: root's directories,
// a USB stick mounted at /mnt/sda1, and /mnt/evil, a link to /etc.
func router(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"etc", "data", "tmp", "mnt/sda1", "usr/share/verso"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("../etc", filepath.Join(root, "mnt/evil")); err != nil {
		t.Fatal(err)
	}
	return root
}

// runHistoryDir runs history_dir under busybox sh, the shell OpenWrt runs the
// init script in, with directory configured and /data mounted or not. It
// answers the directory handed to the shell and each chown made.
func runHistoryDir(t *testing.T, busybox, root, configured string, dataMounted bool) (string, []string) {
	t.Helper()
	mounts := "/dev/root /rom squashfs ro 0 0\n"
	if dataMounted {
		mounts += "/dev/mmcblk0p5 /data ext4 rw 0 0\n"
	}
	scratch := t.TempDir()
	mountsFile, chownsFile := filepath.Join(scratch, "mounts"), filepath.Join(scratch, "chowns")
	if err := os.WriteFile(mountsFile, []byte(mounts), 0o644); err != nil {
		t.Fatal(err)
	}
	script := historyDirHarness + historyDirFunction(t) + "history_dir\n"
	cmd := exec.Command(busybox, "sh", "-c", script, "sh", root, configured, mountsFile, chownsFile, busybox)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("history_dir: %v: %s", err, stderr.String())
	}
	chowns, err := os.ReadFile(chownsFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(out), "\n"), strings.FieldsFunc(string(chowns), func(r rune) bool { return r == '\n' })
}

// TestHistoryDirHandsOverOnlyWhatRootMakesOrTheDefault: the directory is the
// verso user's to name, so root gives it a directory it has just made, one
// name into /data or /tmp, two into /mnt, through no link, and the defaults,
// below root's own directories, even when they stand already, as a
// sysupgrade that keeps settings brings them back as root's. Whatever is
// named is handed to the shell, which keeps in memory a directory it cannot
// write.
func TestHistoryDirHandsOverOnlyWhatRootMakesOrTheDefault(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("history_dir runs under busybox sh, which is not installed")
	}
	cases := []struct {
		name        string
		configured  string
		dataMounted bool
		standing    []string          // directories there before the start
		links       map[string]string // links there before the start
		files       []string          // files there before the start
		want        string
		chowned     bool
	}{
		{name: "default on the overlay", want: "/usr/share/verso/history", chowned: true},
		{name: "default on the overlay a sysupgrade brought back as root's", standing: []string{"usr/share/verso/history"}, want: "/usr/share/verso/history", chowned: true},
		{name: "default on /data", dataMounted: true, want: "/data/verso", chowned: true},
		{name: "default on /data standing", dataMounted: true, standing: []string{"data/verso"}, want: "/data/verso", chowned: true},
		{name: "/data/verso named", configured: "/data/verso", standing: []string{"data/verso"}, want: "/data/verso", chowned: true},
		{name: "one name into /data", configured: "/data/history", want: "/data/history", chowned: true},
		{name: "one name into /tmp", configured: "/tmp/history", want: "/tmp/history", chowned: true},
		{name: "on a stick", configured: "/mnt/sda1/verso", want: "/mnt/sda1/verso", chowned: true},
		{name: "on a stick standing", configured: "/mnt/sda1/verso", standing: []string{"mnt/sda1/verso"}, want: "/mnt/sda1/verso"},
		{name: "one name into /tmp standing", configured: "/tmp/history", standing: []string{"tmp/history"}, want: "/tmp/history"},
		{name: "one name into /data standing", configured: "/data/history", standing: []string{"data/history"}, want: "/data/history"},
		{name: "/etc", configured: "/etc", want: "/etc"},
		{name: "into /etc", configured: "/etc/verso", want: "/etc/verso"},
		{name: "/data itself", configured: "/data", want: "/data"},
		{name: "deeper into /data", configured: "/data/a/b", want: "/data/a/b"},
		{name: "deeper into /tmp", configured: "/tmp/a/b", want: "/tmp/a/b"},
		{name: "/mnt itself", configured: "/mnt/sda1", want: "/mnt/sda1"},
		{name: "deeper on a stick", configured: "/mnt/sda1/a/b", want: "/mnt/sda1/a/b"},
		{name: "below the default", configured: "/usr/share/verso/history/x", want: "/usr/share/verso/history/x"},
		{name: "beside the default", configured: "/usr/share/verso/other", want: "/usr/share/verso/other"},
		{name: "default with a trailing slash", configured: "/data/verso/", standing: []string{"data/verso"}, want: "/data/verso/"},
		{name: "through a linked mount", configured: "/mnt/evil/verso", want: "/mnt/evil/verso"},
		{name: "default as a link", links: map[string]string{"usr/share/verso/history": "/etc"}, want: "/usr/share/verso/history"},
		{name: "default on /data as a link", dataMounted: true, links: map[string]string{"data/verso": "../etc"}, want: "/data/verso"},
		{name: "default on /data through a link", configured: "/data/verso", links: map[string]string{"data": "etc"}, want: "/data/verso"},
		{name: "default as a file", files: []string{"usr/share/verso/history"}, want: "/usr/share/verso/history"},
		{name: "default on /data as a file", dataMounted: true, files: []string{"data/verso"}, want: "/data/verso"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := router(t)
			for _, dir := range c.standing {
				if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range c.files {
				if err := os.WriteFile(filepath.Join(root, file), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for link, target := range c.links {
				_ = os.Remove(filepath.Join(root, link))
				if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.Lstat(filepath.Join(root, c.want))
			got, chowns := runHistoryDir(t, busybox, root, c.configured, c.dataMounted)
			if got != c.want {
				t.Errorf("history_dir = %q, want %q", got, c.want)
			}
			var want []string
			if c.chowned {
				want = []string{"-h verso:verso " + c.want}
			}
			if !slices.Equal(chowns, want) {
				t.Errorf("chowns %q, want %q", chowns, want)
			}
			after, err := os.Lstat(filepath.Join(root, c.want))
			switch {
			case before == nil && c.chowned:
				if err != nil || !after.IsDir() || after.Mode().Perm() != 0o700 {
					t.Errorf("made %v (%v), want a directory 0700", after, err)
				}
			case before == nil && err == nil:
				t.Errorf("made %s, want nothing made", c.want)
			}
			if entries, err := os.ReadDir(filepath.Join(root, "etc")); err != nil || len(entries) != 0 {
				t.Errorf("/etc holds %v (%v), want nothing made there", entries, err)
			}
		})
	}
}

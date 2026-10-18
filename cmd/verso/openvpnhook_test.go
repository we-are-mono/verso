// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// openvpnHook is the hook the verso package ships for OpenWrt's hotplug-call
// to run on each OpenVPN instance's transitions.
const openvpnHook = "../../docker/rootfs/etc/hotplug.d/openvpn/50-verso"

// openvpnHookRun runs the hook as hotplug-call does, sourced in a subshell,
// once $PLANT ("fifo" or "file") has stood at .vpn1.<the shell's PID> in the
// record directory, the name a hook taking its PID for a temporary name would
// write. The name planted and its inode go to $PLANTED.
const openvpnHookRun = `D=$1 HOOK=$2
case $PLANT in
	fifo) mkfifo "$D/.vpn1.$$" ;;
	file) : >"$D/.vpn1.$$" ;;
esac
[ -n "$PLANT" ] && echo ".vpn1.$$ $(stat -c %i "$D/.vpn1.$$")" >"$PLANTED"
( . "$HOOK" )
`

// hookTimeout bounds a run of the hook: one blocked on a FIFO never ends.
const hookTimeout = 5 * time.Second

// openvpnRecords is a record directory as the hook makes it, 1733, and a copy
// of the hook pointed at it. It answers the directory and the hook's path.
func openvpnRecords(t *testing.T) (string, string) {
	t.Helper()
	scratch, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(scratch, "records")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o1733); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(openvpnHook)
	if err != nil {
		t.Fatal(err)
	}
	const named = "dir=/var/run/verso-openvpn\n"
	if !bytes.Contains(script, []byte(named)) {
		t.Fatalf("%s names no record directory", openvpnHook)
	}
	hook := filepath.Join(scratch, "50-verso")
	script = bytes.Replace(script, []byte(named), []byte("dir="+dir+"\n"), 1)
	if err := os.WriteFile(hook, script, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, hook
}

// busyboxPath is a directory of the commands the hook and its harness call,
// each busybox, as on the router.
func busyboxPath(t *testing.T, busybox string) string {
	t.Helper()
	bin := t.TempDir()
	for _, applet := range []string{"chmod", "cp", "date", "id", "ls", "mkdir", "mkfifo", "mktemp", "mv", "rm", "stat"} {
		if err := os.Symlink(busybox, filepath.Join(bin, applet)); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

// recordOf is the name the hook writes instance's record under when it runs
// as uid: the instance's name for root, the name and the uid for the user
// an OpenVPN drops to.
func recordOf(instance string, uid int) string {
	if uid == 0 {
		return instance
	}
	return instance + "." + strconv.Itoa(uid)
}

// planted is what the harness stood in the record directory: its name and
// inode.
type planted struct {
	name  string
	inode uint64
}

// runOpenVPNHook runs the hook for instance vpn1's up under busybox sh, the
// shell hotplug-call runs it in, plant ("", "fifo" or "file") standing first
// at the name a PID-named temporary would take. A hook still running at the
// timeout is killed and fails the test. It answers what was planted.
func runOpenVPNHook(t *testing.T, busybox, dir, hook, plant string) planted {
	t.Helper()
	plantedFile := filepath.Join(t.TempDir(), "planted")
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, busybox, "sh", "-c", openvpnHookRun, "sh", dir, hook)
	cmd.Env = []string{
		"PATH=" + busyboxPath(t, busybox),
		"PLANT=" + plant,
		"PLANTED=" + plantedFile,
		"INSTANCE=vpn1",
		"ACTION=up",
		"dev=tun0",
		"ifconfig_local=10.8.0.2",
		"trusted_ip=192.0.2.1",
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("the hook still ran after %v: the planted %s blocked it", hookTimeout, plant)
	}
	if err != nil {
		t.Fatalf("hook: %v: %s", err, out)
	}
	if plant == "" {
		return planted{}
	}
	line, err := os.ReadFile(plantedFile)
	if err != nil {
		t.Fatal(err)
	}
	name, inode, _ := strings.Cut(strings.TrimSpace(string(line)), " ")
	n, err := strconv.ParseUint(inode, 10, 64)
	if err != nil {
		t.Fatalf("planted %q: %v", line, err)
	}
	return planted{name: name, inode: n}
}

// TestOpenVPNHookWritesItsRecordPastWhatOthersPlant: the record directory
// takes anyone's file, so whatever another account stands at a name the hook
// could guess neither holds the hook up (a FIFO would block its write
// forever, and OpenVPN with it) nor becomes the record (a file of theirs the
// hook wrote into and renamed in place). The record is a file the hook made,
// carrying the PID of the OpenVPN that ran it, and the hook leaves no
// temporary of its own behind.
func TestOpenVPNHookWritesItsRecordPastWhatOthersPlant(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("the hook runs under busybox sh, which is not installed")
	}
	for _, plant := range []string{"", "fifo", "file"} {
		t.Run("planted "+plant, func(t *testing.T) {
			dir, hook := openvpnRecords(t)
			stood := runOpenVPNHook(t, busybox, dir, hook, plant)

			name := recordOf("vpn1", os.Getuid())
			record := filepath.Join(dir, name)
			info, err := os.Lstat(record)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("record %v (%v), want a regular file", info, err)
			}
			if ino := info.Sys().(*syscall.Stat_t).Ino; stood.name != "" && ino == stood.inode {
				t.Fatalf("the record is the planted %s (inode %d)", stood.name, ino)
			}
			text, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			pid := "pid=" + strconv.Itoa(os.Getpid())
			for _, line := range []string{pid, "state=connecting", "address=10.8.0.2", "server=192.0.2.1:1194", "device=tun0"} {
				if !strings.Contains(string(text), line+"\n") {
					t.Errorf("record %q holds no %q", text, line)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if e.Name() != name && e.Name() != stood.name {
					t.Errorf("the hook left %s behind", e.Name())
				}
			}
		})
	}
}

// TestOpenVPNHookRemovesItsTemporaryWhenTheRecordCannotBeReplaced: a
// directory standing at the record's name, even one open to the hook, is not
// a place to rename the record into; the hook leaves both directories as it
// found them.
func TestOpenVPNHookRemovesItsTemporaryWhenTheRecordCannotBeReplaced(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("the hook runs under busybox sh, which is not installed")
	}
	dir, hook := openvpnRecords(t)
	name := recordOf("vpn1", os.Getuid())
	stood := filepath.Join(dir, name)
	if err := os.Mkdir(stood, 0o777); err != nil {
		t.Fatal(err)
	}
	runOpenVPNHook(t, busybox, dir, hook, "")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		t.Errorf("record directory holds %v, want only %s", entries, name)
	}
	inside, err := os.ReadDir(stood)
	if err != nil {
		t.Fatal(err)
	}
	if len(inside) != 0 {
		t.Errorf("the directory at the record's name took %v", inside)
	}
}

// openvpnHookAfterRecord stands a record at vpn1, carrying the PID of the
// OpenVPN about to run the hook and a since, server and address of the
// writer's choosing, then runs the hook as hotplug-call does. It runs as root
// of a user namespace that maps uid $PLANTER too: given one, the writer drops
// to it from inside the record directory, as an account with no way to the
// directory's ancestors still writes into it. Given $TARGET, the record is
// written there and vpn1 is a link to it.
const openvpnHookAfterRecord = `D=$1 HOOK=$2
PATH=$HOOKPATH
cd "$D" || exit 1
plant='printf "pid=%s\nstate=connected\nsince=1\naddress=6.6.6.6\nserver=evil.example:443\ndevice=evil0\n" "$1" >"${TARGET:-vpn1}" &&
	{ [ -z "$TARGET" ] || "$BUSYBOX" ln -s "$TARGET" vpn1; }'
if [ -n "$PLANTER" ]; then
	"$SETPRIV" --reuid "$PLANTER" --regid "$PLANTER" --clear-groups "$BUSYBOX" sh -c "$plant" sh "$PPID"
else
	"$BUSYBOX" sh -c "$plant" sh "$PPID"
fi || exit 1
( . "$HOOK" )
`

// userNamespace is the unshare and setpriv a test runs the hook with as root
// of a user namespace that maps uids 1001 and 1002 too, and the unshare
// arguments that make one; the test is skipped where there is none.
func userNamespace(t *testing.T) (unshare, setpriv string, userns []string) {
	t.Helper()
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("another account's record needs unshare, which is not installed")
	}
	setpriv, err = exec.LookPath("setpriv")
	if err != nil {
		t.Skip("another account's record needs setpriv, which is not installed")
	}
	userns = []string{"--map-root-user", "--map-auto"}
	for _, uid := range []string{"1001", "1002"} {
		probe := exec.Command(unshare, append(userns, setpriv, "--reuid", uid, "--regid", uid, "--clear-groups", "true")...)
		if out, err := probe.CombinedOutput(); err != nil {
			t.Skipf("no user namespace mapping uid %s: %v: %s", uid, err, out)
		}
	}
	return unshare, setpriv, userns
}

// TestOpenVPNHookCarriesOverOnlyItsOwnRecord: what an instance carries from
// one hook to the next (since, address, server) comes only from a record the
// hook's own user wrote. A record another account stood at the instance's
// name before the first hook ran, with the right PID, would otherwise pass its
// server and address into the record the hook writes, which the reader trusts
// as the instance's own.
func TestOpenVPNHookCarriesOverOnlyItsOwnRecord(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("the hook runs under busybox sh, which is not installed")
	}
	unshare, setpriv, userns := userNamespace(t)

	for _, tc := range []struct {
		name, planter, target string
		carried               []string
	}{
		{"the hook's own", "", "", []string{"since=1", "address=6.6.6.6", "server=evil.example:443"}},
		{"another account's", "1001", "", []string{"since=", "address=", "server="}},
		{"a link to the hook's own", "", "vpn1.real", []string{"since=", "address=", "server="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, hook := openvpnRecords(t)
			ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
			defer cancel()
			args := append(append([]string{}, userns...), busybox, "sh", "-c", openvpnHookAfterRecord, "sh", dir, hook)
			cmd := exec.CommandContext(ctx, unshare, args...)
			cmd.Env = []string{
				"PATH=" + os.Getenv("PATH"),
				"HOOKPATH=" + busyboxPath(t, busybox),
				"BUSYBOX=" + busybox,
				"SETPRIV=" + setpriv,
				"PLANTER=" + tc.planter,
				"TARGET=" + tc.target,
				"INSTANCE=vpn1",
				"ACTION=up",
				"dev=tun0",
				"trusted_ip6=2001:db8::1",
			}
			cmd.WaitDelay = time.Second
			if out, err := cmd.CombinedOutput(); err != nil || ctx.Err() != nil {
				t.Fatalf("hook: %v (%v): %s", err, ctx.Err(), out)
			}

			record := filepath.Join(dir, "vpn1")
			info, err := os.Lstat(record)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("record %v (%v), want a regular file", info, err)
			}
			if uid := info.Sys().(*syscall.Stat_t).Uid; uid != uint32(os.Getuid()) {
				t.Errorf("record owned by %d, want the hook's own %d", uid, os.Getuid())
			}
			text, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range append([]string{"state=connecting"}, append(tc.carried, "device=tun0")...) {
				if !strings.Contains(string(text), "\n"+line+"\n") {
					t.Errorf("record %q holds no %q", text, line)
				}
			}
		})
	}
}

// openvpnHookDropped runs the hook as an OpenVPN that drops to uid 1001
// runs it, every run a child of this shell so each carries its PID: as root
// for route-up, as 1001 for route-pre-down and an ipchange after. Given
// $PLANTER, root's route-up does not run and that account stands a record at
// the instance's name first, carrying the PID and a server of its choosing.
const openvpnHookDropped = `D=$1 HOOK=$2
PATH=$HOOKPATH
cd "$D" || exit 1
hook='. "$1"'
if [ -n "$PLANTER" ]; then
	"$SETPRIV" --reuid "$PLANTER" --regid "$PLANTER" --clear-groups "$BUSYBOX" sh -c \
		'printf "pid=%s\nstate=connected\nsince=1\naddress=6.6.6.6\nserver=evil.example:443\n" "$1" >vpn1' sh "$$" || exit 1
else
	ACTION=route-up dev=tun0 ifconfig_local=10.8.0.2 trusted_ip=192.0.2.1 trusted_port=1194 \
		"$BUSYBOX" sh -c "$hook" sh "$HOOK" || exit 1
fi
as1001() { "$SETPRIV" --reuid 1001 --regid 1001 --clear-groups "$BUSYBOX" env "$@" "$BUSYBOX" sh -c "$hook" sh "$HOOK"; }
as1001 ACTION=route-pre-down || exit 1
cp vpn1.1001 "$OUT/pre-down" || exit 1
as1001 ACTION=ipchange trusted_ip=192.0.2.9 trusted_port=1194 || exit 1
`

// TestOpenVPNHookAfterDroppingPrivilegesCarriesRootsRecord: an OpenVPN with
// a profile's user runs route-up as root and the hooks after as that user,
// which cannot replace root's record in the 1733 directory. The user's hooks
// write the instance's record under its name and their uid, carrying over
// since, address and server from root's record of the same run, so a
// reconnect and a server change still reach the reader. A record at root's
// name that another account wrote is carried over by no one.
func TestOpenVPNHookAfterDroppingPrivilegesCarriesRootsRecord(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("the hook runs under busybox sh, which is not installed")
	}
	unshare, setpriv, userns := userNamespace(t)

	for _, tc := range []struct {
		name, planter      string
		preDown, ipchanged []string
	}{
		{
			"root's", "",
			[]string{"state=connecting", "address=10.8.0.2", "server=192.0.2.1:1194", "device=tun0"},
			[]string{"state=connecting", "address=10.8.0.2", "server=192.0.2.9:1194", "device=tun0"},
		},
		{
			"another account's", "1002",
			[]string{"state=connecting", "since=", "address=", "server=", "device="},
			[]string{"state=connecting", "since=", "address=", "server=192.0.2.9:1194", "device="},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, hook := openvpnRecords(t)
			// The user OpenVPN drops to reaches the hook and the record
			// directory by their paths, as on the router.
			for at := filepath.Dir(dir); at != filepath.Dir(filepath.Dir(filepath.Dir(dir))); at = filepath.Dir(at) {
				if err := os.Chmod(at, 0o711); err != nil {
					t.Fatal(err)
				}
			}
			out := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
			defer cancel()
			args := append(append([]string{}, userns...), busybox, "sh", "-c", openvpnHookDropped, "sh", dir, hook)
			cmd := exec.CommandContext(ctx, unshare, args...)
			cmd.Env = []string{
				"PATH=" + os.Getenv("PATH"),
				"HOOKPATH=" + busyboxPath(t, busybox),
				"BUSYBOX=" + busybox,
				"SETPRIV=" + setpriv,
				"PLANTER=" + tc.planter,
				"OUT=" + out,
				"INSTANCE=vpn1",
			}
			cmd.WaitDelay = time.Second
			if out, err := cmd.CombinedOutput(); err != nil || ctx.Err() != nil {
				t.Fatalf("hook: %v (%v): %s", err, ctx.Err(), out)
			}

			roots, err := os.ReadFile(filepath.Join(dir, "vpn1"))
			if err != nil {
				t.Fatal(err)
			}
			since := ""
			for _, line := range strings.Split(string(roots), "\n") {
				if s, ok := strings.CutPrefix(line, "since="); ok {
					since = s
				}
			}
			if tc.planter == "" {
				if !strings.Contains(string(roots), "\nstate=connected\n") || since == "" {
					t.Fatalf("root's route-up record %q", roots)
				}
				tc.preDown = append(tc.preDown, "since="+since)
				tc.ipchanged = append(tc.ipchanged, "since="+since)
			}
			info, err := os.Lstat(filepath.Join(dir, "vpn1.1001"))
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("the user's record %v (%v), want a regular file", info, err)
			}
			if uid := info.Sys().(*syscall.Stat_t).Uid; uid == uint32(os.Getuid()) {
				t.Errorf("the user's record is owned by root of the namespace (%d)", uid)
			}
			for file, want := range map[string][]string{filepath.Join(out, "pre-down"): tc.preDown, filepath.Join(dir, "vpn1.1001"): tc.ipchanged} {
				text, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				for _, line := range want {
					if !strings.Contains(string(text), "\n"+line+"\n") {
						t.Errorf("%s: record %q holds no %q", filepath.Base(file), text, line)
					}
				}
				if strings.Contains(string(text), "evil") {
					t.Errorf("%s: record %q carried another account's", filepath.Base(file), text)
				}
			}
		})
	}
}

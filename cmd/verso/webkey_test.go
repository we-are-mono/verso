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

// rpcdInitScript is the script procd starts verso-rpcd through.
const rpcdInitScript = "../../docker/rootfs/etc/init.d/verso-rpcd"

// TestCloseWebKeyGivesOnlyAFileToTheShell: the verso-rpcd init script makes
// the web certificate's key the shell's account's alone, by uid since a
// restored /etc/passwd may not name it, and leaves a link there as it stands,
// since chmod and chown would reach what it names, /etc/shadow say.
func TestCloseWebKeyGivesOnlyAFileToTheShell(t *testing.T) {
	busybox, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("close_web_key runs under busybox sh, which is not installed")
	}
	for _, c := range []struct {
		name  string
		make  func(t *testing.T, key string)
		calls []string
	}{
		{"a key", func(t *testing.T, key string) { writeKey(t, key) }, []string{"chmod 0600 KEY", "chown 6000:0 KEY"}},
		{"no key", func(*testing.T, string) {}, nil},
		{"a link to a file", func(t *testing.T, key string) {
			target := filepath.Join(filepath.Dir(key), "shadow")
			writeKey(t, target)
			if err := os.Symlink(target, key); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"a directory", func(t *testing.T, key string) {
			if err := os.Mkdir(key, 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			scratch := t.TempDir()
			key, calls := filepath.Join(scratch, "tls.key"), filepath.Join(scratch, "calls")
			c.make(t, key)
			// chmod and chown are written to $CALLS rather than made.
			script := `CALLS=$1
chmod() { echo "chmod $*" >>"$CALLS"; }
chown() { echo "chown $*" >>"$CALLS"; }
` + initFunction(t, rpcdInitScript, "close_web_key") + `close_web_key "$2"` + "\n"
			var stderr bytes.Buffer
			cmd := exec.Command(busybox, "sh", "-c", script, "sh", calls, key)
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("close_web_key: %v: %s", err, stderr.String())
			}
			made, err := os.ReadFile(calls)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			got := strings.FieldsFunc(strings.ReplaceAll(string(made), key, "KEY"), func(r rune) bool { return r == '\n' })
			if !slices.Equal(got, c.calls) {
				t.Errorf("calls %q, want %q", got, c.calls)
			}
		})
	}
}

// writeKey writes a file at path, group-readable as an earlier release left
// the key.
func writeKey(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("key"), 0o640); err != nil {
		t.Fatal(err)
	}
}

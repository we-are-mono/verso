// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build linux

package peercred

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestUIDIsTheListenersAccount: the uid read from a dialled socket is the
// account that listens behind it, here this test's own.
func TestUIDIsTheListenersAccount(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	uid, err := UID(conn)
	if err != nil {
		t.Fatalf("UID: %v", err)
	}
	if want := uint32(os.Getuid()); uid != want {
		t.Errorf("UID = %d, want %d", uid, want)
	}
}

// TestUIDRefusesWhatIsNoUnixSocket: a connection the kernel keeps no
// credentials for names no account, rather than a zero that reads as root.
func TestUIDRefusesWhatIsNoUnixSocket(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	if uid, err := UID(client); err == nil {
		t.Fatalf("UID of a pipe = %d, want an error", uid)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortRestartWindow keeps a test's wait for a helper that never comes back
// short.
func shortRestartWindow(t *testing.T) {
	t.Helper()
	previous := helperRestartWindow
	helperRestartWindow = 600 * time.Millisecond
	t.Cleanup(func() { helperRestartWindow = previous })
}

// serveHelper answers each accepted connection with the next reply; an empty
// reply reads the request and hangs up without answering, as a helper going
// down mid-request does. It reports how many connections it took.
func serveHelper(listener net.Listener, replies ...string) <-chan int {
	taken := make(chan int, 1)
	go func() {
		n := 0
		defer func() { taken <- n }()
		for _, reply := range replies {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			n++
			_, _ = bufio.NewReader(conn).ReadString('\n')
			if reply != "" {
				_, _ = conn.Write([]byte(reply + "\n"))
			}
			_ = conn.Close()
		}
	}()
	return taken
}

// TestAReadOutlastsAHelperRestart: upgrading Verso restarts verso-rpcd, and a
// page drawn in that moment asks a helper that hangs up without answering. A
// read asks again, so the page reads the router rather than "EOF".
func TestAReadOutlastsAHelperRestart(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener := listenAsHelper(t, socket)
	defer listener.Close()
	taken := serveHelper(listener, "", `{"status":0,"result":{"packages":[{"name":"verso","version":"0.11.0-r0","installed":true}]}}`)
	packages, err := (&NativeBackend{helperSocket: socket}).PkgInstalled(context.Background(), "sid")
	if err != nil || len(packages) != 1 || packages[0].Version != "0.11.0-r0" {
		t.Fatalf("packages = %+v, err = %v", packages, err)
	}
	listener.Close()
	if n := <-taken; n != 2 {
		t.Fatalf("connections = %d, want 2", n)
	}
}

// TestACallWaitsForTheHelperToListenAgain: between the old helper's exit and
// the new one's bind there is no socket at all. Nothing has been sent, so any
// call, a change included, waits for the helper to listen again.
func TestACallWaitsForTheHelperToListenAgain(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	expectHelperOwner(t, uint32(os.Getuid()))
	bound := make(chan net.Listener, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		listener, err := net.Listen("unix", socket)
		if err != nil {
			bound <- nil
			return
		}
		bound <- listener
		serveHelper(listener, `{"status":0}`)
	}()
	if err := (&NativeBackend{helperSocket: socket}).PkgInstall(context.Background(), "sid", "htop"); err != nil {
		t.Fatal(err)
	}
	if listener := <-bound; listener != nil {
		listener.Close()
	}
}

// TestAChangeIsNeverSentTwice: a helper that hangs up after a change was sent
// may have made it, so the change is not asked again: the hang-up is the
// answer.
func TestAChangeIsNeverSentTwice(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener := listenAsHelper(t, socket)
	defer listener.Close()
	taken := serveHelper(listener, "", `{"status":0}`)
	if err := (&NativeBackend{helperSocket: socket}).PkgInstall(context.Background(), "sid", "htop"); err == nil {
		t.Fatal("a hang-up after a change read as success")
	}
	listener.Close()
	if n := <-taken; n != 1 {
		t.Fatalf("connections = %d, want 1", n)
	}
}

// TestAReadGivesUpOnAHelperThatStaysAway: a helper that does not come back
// within the window is reported, not waited on for ever.
func TestAReadGivesUpOnAHelperThatStaysAway(t *testing.T) {
	shortRestartWindow(t)
	expectHelperOwner(t, uint32(os.Getuid()))
	start := time.Now()
	_, err := (&NativeBackend{helperSocket: filepath.Join(t.TempDir(), "absent.sock")}).PkgInstalled(context.Background(), "sid")
	if err == nil {
		t.Fatal("an absent helper answered")
	}
	if waited := time.Since(start); waited < 500*time.Millisecond || waited > 3*time.Second {
		t.Fatalf("waited %v for a helper that stayed away", waited)
	}
}

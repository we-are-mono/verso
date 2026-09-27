// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestServeProcess(t *testing.T) {
	if os.Getenv("VERSO_TEST_SERVE") == "1" {
		serve()
	}
}

func serveProcess(t *testing.T, addr string) (*exec.Cmd, context.CancelFunc) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestServeProcess$")
	cmd.Env = append(os.Environ(), "VERSO_TEST_SERVE=1", "VERSO_ADDR="+addr,
		"VERSO_PLUGINS_DIR="+t.TempDir(), "VERSO_I18N_DIR="+t.TempDir())
	return cmd, cancel
}

func TestStartupMessagesUseStdout(t *testing.T) {
	cmd, cancel := serveProcess(t, "127.0.0.1:0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var lines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.HasPrefix(line, "verso listening on ") {
			break
		}
	}
	cancel()
	_ = cmd.Wait()
	if stderr.Len() != 0 {
		t.Fatalf("successful startup wrote to procd's error stream: %s", stderr.String())
	}
	for i, prefix := range []string{"verso: discovered 0 plugin(s)", "verso: loaded 0 language(s)", "verso listening on 127.0.0.1:"} {
		if i >= len(lines) || !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("stdout missing untimestamped startup message %q: %v", prefix, lines)
		}
	}
}

// TestTermStopsTheShellCleanly: procd stops a service with SIGTERM, and the
// shell answers it by closing down — its background work stopped and, under
// scripts/dev.sh, its sessions left for the next shell — rather than dying
// mid-flight. A clean stop exits 0.
func TestTermStopsTheShellCleanly(t *testing.T) {
	cmd, _ := serveProcess(t, "127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "verso listening on ") {
			break
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	go func() {
		for scanner.Scan() {
		}
	}()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("SIGTERM must stop the shell cleanly, got %v", err)
	}
}

func TestBindFailureUsesStderr(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	cmd, _ := serveProcess(t, occupied.Addr().String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("binding an occupied port must fail")
	}
	if !strings.Contains(stderr.String(), "address already in use") {
		t.Fatalf("bind failure missing from stderr: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "verso listening on") {
		t.Fatalf("failed bind announced successful startup: %s", stdout.String())
	}
}

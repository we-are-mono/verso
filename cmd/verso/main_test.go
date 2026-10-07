// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/tlscert"
)

func TestServeProcess(t *testing.T) {
	if os.Getenv("VERSO_TEST_SERVE") == "1" {
		serve(strings.Fields(os.Getenv("VERSO_TEST_ARGS")))
	}
}

// serveProcess runs the shell with HTTPS on addr, HTTP beside it on any free
// port, and a certificate named "first" in tlsDir. args are the shell's own.
func serveProcess(t *testing.T, addr string, args ...string) (*exec.Cmd, context.CancelFunc) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tlsDir := t.TempDir()
	writePair(t, tlsDir, "first")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestServeProcess$")
	args = append([]string{"-listen-https", addr, "-listen-http", "127.0.0.1:0"}, args...)
	cmd.Env = append(os.Environ(), "VERSO_TEST_SERVE=1", "VERSO_TEST_ARGS="+strings.Join(args, " "),
		"VERSO_TLS_DIR="+tlsDir, "VERSO_PLUGINS_DIR="+t.TempDir(), "VERSO_I18N_DIR="+t.TempDir())
	return cmd, cancel
}

func writePair(t *testing.T, dir, name string) {
	t.Helper()
	cert, key, err := tlscert.Generate([]string{name}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tlscert.Write(dir, cert, key); err != nil {
		t.Fatal(err)
	}
}

// started runs cmd until it has announced every listener, returning the
// address each scheme listens on and the rest of its output, line by line.
func started(t *testing.T, cmd *exec.Cmd) (map[string]string, <-chan string) {
	t.Helper()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	addrs := map[string]string{}
	scanner := bufio.NewScanner(stdout)
	for len(addrs) < 2 && scanner.Scan() {
		if rest, ok := strings.CutPrefix(scanner.Text(), "verso listening on "); ok {
			addr, scheme, _ := strings.Cut(rest, " ")
			addrs[strings.Trim(scheme, "()")] = addr
		}
	}
	rest := make(chan string, 16)
	go func() {
		for scanner.Scan() {
			rest <- scanner.Text()
		}
		close(rest)
	}()
	return addrs, rest
}

// served is the subject of the certificate the HTTPS listener presents.
func served(t *testing.T, addr string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0].Subject.CommonName
}

func TestHTTPSServesTheCertificateOnDiskAndHTTPSendsBrowsersThere(t *testing.T) {
	cmd, cancel := serveProcess(t, "127.0.0.1:0")
	addrs, _ := started(t, cmd)
	defer func() { cancel(); _ = cmd.Wait() }()
	if got := served(t, addrs["https"]); got != "first" {
		t.Errorf("HTTPS presents %q", got)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + addrs["http"] + "/system/access")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	_, port, _ := net.SplitHostPort(addrs["https"])
	if want := "https://127.0.0.1:" + port + "/system/access"; resp.StatusCode != http.StatusTemporaryRedirect || resp.Header.Get("Location") != want {
		t.Errorf("HTTP answers %d %q, want a redirect to %q", resp.StatusCode, resp.Header.Get("Location"), want)
	}
}

func TestWithTheRedirectOffHTTPServesTheShell(t *testing.T) {
	cmd, cancel := serveProcess(t, "127.0.0.1:0", "-redirect-https", "0")
	addrs, _ := started(t, cmd)
	defer func() { cancel(); _ = cmd.Wait() }()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + addrs["http"] + "/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusTemporaryRedirect {
		t.Errorf("HTTP redirected to %q with the redirect off", resp.Header.Get("Location"))
	}
}

// TestHUPServesTheCertificateWrittenSince: the certificate's writer has procd
// send SIGHUP once it is replaced, and the next handshake presents the new one.
func TestHUPServesTheCertificateWrittenSince(t *testing.T) {
	cmd, cancel := serveProcess(t, "127.0.0.1:0")
	addrs, rest := started(t, cmd)
	defer func() { cancel(); _ = cmd.Wait() }()
	var tlsDir string
	for _, kv := range cmd.Env {
		if v, ok := strings.CutPrefix(kv, "VERSO_TLS_DIR="); ok {
			tlsDir = v
		}
	}
	writePair(t, tlsDir, "second")
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	for line := range rest {
		if line == "verso: certificate reloaded" {
			break
		}
	}
	if got := served(t, addrs["https"]); got != "second" {
		t.Errorf("after SIGHUP HTTPS presents %q", got)
	}
}

func TestWithNoCertificateTheShellRefusesToStart(t *testing.T) {
	cmd, _ := serveProcess(t, "127.0.0.1:0")
	cmd.Env = append(cmd.Env, "VERSO_TLS_DIR="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("the shell started with no certificate")
	}
	if !strings.Contains(stderr.String(), "no certificate to serve HTTPS with") {
		t.Errorf("stderr: %s", stderr.String())
	}
}

func TestAListenerThatIsNoAddressStopsTheShell(t *testing.T) {
	cmd, _ := serveProcess(t, "router:443")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("the shell started on a listener that is no address")
	}
	if !strings.Contains(stderr.String(), `listener "router:443" is not address:port`) {
		t.Errorf("stderr: %s", stderr.String())
	}
}

func TestCertificateGenerateNamesWhatTheLANUses(t *testing.T) {
	dir := t.TempDir()
	if err := certificate([]string{"generate", dir, "verso-lab", "verso-lab.lan", "192.168.1.1", "2a02:1::1", "fd00::1"}); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, tlscert.CertFile), filepath.Join(dir, tlscert.KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(pair.Leaf.DNSNames, pair.Leaf.IPAddresses) != "[verso-lab verso-lab.lan] [192.168.1.1 fd00::1]" {
		t.Errorf("names: %v %v", pair.Leaf.DNSNames, pair.Leaf.IPAddresses)
	}
	if err := certificate([]string{"generate"}); err == nil {
		t.Error("a generate with no directory succeeded")
	}
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

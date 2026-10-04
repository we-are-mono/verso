// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestBundledPluginPages exercises the built plugins through the real socket
// transport and shell renderer. Router reads are fake; no router is required.
// Run after make build: VERSO_BUNDLED_SMOKE=1 go test ./internal/server -run TestBundledPluginPages.
func TestBundledPluginPages(t *testing.T) {
	if os.Getenv("VERSO_BUNDLED_SMOKE") != "1" {
		t.Skip("requires built amd64 plugin binaries")
	}
	manifests, err := filepath.Glob("../../plugins/*/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var installed []plugin.Manifest
	for _, name := range manifests {
		dir := filepath.Dir(name)
		if _, err := os.Stat(filepath.Join(dir, "bundled")); err != nil {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var m plugin.Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		m.Socket = filepath.Join(t.TempDir(), m.ID+".sock")
		binary, err := filepath.Abs("../../build/verso-plugin-" + m.ID + "-amd64")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, binary)
		cmd.Env = append(os.Environ(), "VERSO_"+strings.ToUpper(m.ID)+"_SOCKET="+m.Socket)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		ready := false
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(m.Socket); err == nil {
				ready = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("%s did not listen", m.ID)
		}
		installed = append(installed, m)
	}
	if len(installed) == 0 {
		t.Fatal("no bundled plugins discovered")
	}
	backend := fakeBackend{access: true, hn: "router", uci: map[string]map[string]any{
		"system":  {"system": map[string]any{".name": "system", ".type": "system", "hostname": "router", "timezone": "UTC", "zonename": "UTC"}},
		"network": {"lan": map[string]any{".name": "lan", ".type": "interface", "proto": "static", "device": "br-lan", "ipaddr": "192.0.2.1", "netmask": "255.255.255.0"}},
		"dhcp":    {"dnsmasq": map[string]any{".name": "dnsmasq", ".type": "dnsmasq"}},
	}}
	srv := newServerWith(t, backend, plugin.NewSocketTransport(), installed)
	token := srv.sessions.CreateWithMetadata("smoke", "root", "127.0.0.1", "")
	paths := []string{"/", "/devices", "/system/access", "/system/packages", "/system/services", "/system/maintenance", "/system/logs"}
	for _, m := range installed {
		for _, n := range m.Nav {
			paths = append(paths, "/plugins/"+m.ID+n.Path)
		}
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: %d\n%s", path, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "<html") || strings.Contains(rec.Body.String(), "Plugin unavailable") {
				t.Fatalf("page did not render: %s", path)
			}
		})
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command verso is the Verso web UI shell for OpenWrt.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/server"
	"github.com/we-are-mono/verso/internal/widget"
)

func main() {
	addr := os.Getenv("VERSO_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	renderer, err := widget.NewRenderer()
	if err != nil {
		log.Fatalf("verso: %v", err)
	}

	pluginsDir := os.Getenv("VERSO_PLUGINS_DIR")
	if pluginsDir == "" {
		pluginsDir = "/usr/share/verso/plugins"
	}
	manifests, problems := plugin.Discover(os.DirFS(pluginsDir), "*/manifest.json")
	for _, p := range problems {
		log.Printf("verso: %v", p)
	}
	log.Printf("verso: discovered %d plugin(s) in %s", len(manifests), pluginsDir)

	srv, err := server.New(
		renderer,
		openwrt.NewNativeBackend(),
		plugin.NewSocketTransport(),
		manifests,
		openwrt.NewRPCDAuthenticator(),
	)
	if err != nil {
		log.Fatalf("verso: %v", err)
	}
	// The management surface rescans manifests after an install or remove
	// (ADR-011 §7), so a plugin package appears without a shell restart.
	srv.SetRescan(func() []plugin.Manifest {
		rescanned, rescanProblems := plugin.Discover(os.DirFS(pluginsDir), "*/manifest.json")
		for _, p := range rescanProblems {
			log.Printf("verso: %v", p)
		}
		log.Printf("verso: rediscovered %d plugin(s) in %s", len(rescanned), pluginsDir)
		return rescanned
	})
	srv.SetAllowedHosts(allowedHosts())

	log.Printf("verso listening on %s", addr)
	err = http.ListenAndServe(addr, srv.Handler())
	srv.Close()
	if err != nil {
		log.Fatalf("verso: %v", err)
	}
}

// allowedHosts is the DNS-rebinding Host allowlist. It is OPT-IN: unset means an
// empty list, i.e. any Host is accepted (dev convenience). Cross-site writes are
// blocked by the per-session CSRF token (see middleware.go), not by a Host or
// Origin comparison. Setting $VERSO_ALLOWED_HOSTS (comma-sep) turns the rebinding
// guard on for production, with loopback and the device hostname added
// automatically.
func allowedHosts() []string {
	extra := os.Getenv("VERSO_ALLOWED_HOSTS")
	if extra == "" {
		return nil
	}
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if hn, err := os.Hostname(); err == nil && hn != "" {
		hosts = append(hosts, hn)
	}
	return append(hosts, strings.Split(extra, ",")...)
}

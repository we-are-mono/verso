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
		openwrt.NewShadowSecurity(),
	)
	if err != nil {
		log.Fatalf("verso: %v", err)
	}
	srv.SetAllowedHosts(allowedHosts())

	log.Printf("verso listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("verso: %v", err)
	}
}

// allowedHosts is the DNS-rebinding Host allowlist: loopback and the device
// hostname always, plus any hosts/LAN IPs from $VERSO_ALLOWED_HOSTS (comma-sep).
func allowedHosts() []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if hn, err := os.Hostname(); err == nil && hn != "" {
		hosts = append(hosts, hn)
	}
	if extra := os.Getenv("VERSO_ALLOWED_HOSTS"); extra != "" {
		hosts = append(hosts, strings.Split(extra, ",")...)
	}
	return hosts
}

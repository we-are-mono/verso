// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command verso is the Verso web UI shell for OpenWrt.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/server"
	"github.com/we-are-mono/verso/internal/updatecheck"
	"github.com/we-are-mono/verso/internal/widget"
)

func main() {
	// One binary, two jobs. `verso update-check` runs the check the daily cron
	// line schedules and exits; with no arguments the same binary is the web
	// shell. Both reach the same helper verbs and leave the same file behind
	// (ADR-014 §5), which is precisely why they are one program.
	if len(os.Args) > 1 && os.Args[1] == "update-check" {
		if err := updateCheck(); err != nil {
			fmt.Fprintf(os.Stderr, "verso: update-check: %v\n", err)
			os.Exit(1)
		}
		return
	}
	serve()
}

// updateCheck reads both update lanes as local root and records the answer where
// every Verso surface reads it. The identity is rpcd's zero-session convention,
// bounded by verso-rpcd to exactly these two read verbs (ADR-014 §3): nothing
// here can install, apply, or write anything but the state file.
func updateCheck() error {
	truth, err := updatecheck.Run(context.Background(), openwrt.NewNativeBackend(), openwrt.ZeroSID)
	if err != nil {
		return err
	}
	return updatecheck.Write(updatecheck.DefaultDir, truth)
}

func serve() {
	// procd records stdout as info and stderr as err. Keep normal lifecycle
	// messages off the error stream; logd already supplies their timestamp.
	info := log.New(os.Stdout, "", 0)
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
	info.Printf("verso: discovered %d plugin(s) in %s", len(manifests), pluginsDir)

	// Localization catalogs are data packages discovered on disk (ADR-012), the
	// same resilient glob as plugin manifests; a malformed catalog is skipped and
	// reported, never fatal. English needs none.
	i18nDir := os.Getenv("VERSO_I18N_DIR")
	if i18nDir == "" {
		i18nDir = "/usr/share/verso/i18n"
	}
	loadBundle := func() *i18n.Bundle {
		// The shell's catalogs live per component in a per-language directory
		// (<i18nDir>/<code>/base.json); each plugin's travel beside its manifest
		// (<pluginsDir>/<id>/i18n/<code>.json) and override any same-component
		// file in the shell's directory (ADR-012 §1).
		bundle, i18nProblems := i18n.Load(os.DirFS(i18nDir), "*/*.json")
		for _, p := range i18nProblems {
			log.Printf("verso: %v", p)
		}
		for _, p := range bundle.LoadPlugins(os.DirFS(pluginsDir), "*/i18n/*.json") {
			log.Printf("verso: %v", p)
		}
		info.Printf("verso: loaded %d language(s) in %s + %s", len(bundle.Codes()), i18nDir, pluginsDir)
		// The live counterpart of `make i18n-audit`: with VERSO_I18N_RECORD set,
		// every string that falls back to English for an installed language is
		// logged as it renders — including plugin envelope prose the offline
		// audit's fake world never reaches. Browse the pages in that language
		// and the log is the untranslated set.
		if os.Getenv("VERSO_I18N_RECORD") != "" {
			return bundle.Recorded(func(code, component, key string, translated bool) {
				if !translated {
					log.Printf("verso: i18n miss [%s] %s: %q", code, component, key)
				}
			})
		}
		return bundle
	}

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
	// Install the catalogs discovered at startup; English is the built-in source.
	srv.SetBundle(loadBundle())
	// The management surface rescans manifests after an install or remove
	// (ADR-011 §7), so a plugin package appears without a shell restart. A catalog
	// apk lands the same way, so re-read the catalogs on the same trigger — a new
	// language needs no restart either (ADR-012).
	srv.SetRescan(func() []plugin.Manifest {
		rescanned, rescanProblems := plugin.Discover(os.DirFS(pluginsDir), "*/manifest.json")
		for _, p := range rescanProblems {
			log.Printf("verso: %v", p)
		}
		info.Printf("verso: rediscovered %d plugin(s) in %s", len(rescanned), pluginsDir)
		srv.SetBundle(loadBundle())
		return rescanned
	})
	srv.SetAllowedHosts(allowedHosts())

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		srv.Close()
		log.Fatalf("verso: %v", err)
	}
	info.Printf("verso listening on %s", listener.Addr())
	err = http.Serve(listener, srv.Handler())
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

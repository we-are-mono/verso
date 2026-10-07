// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command verso is the Verso web UI shell for OpenWrt.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/listen"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/server"
	"github.com/we-are-mono/verso/internal/tlscert"
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
	// `verso certificate generate` is the init script's, run as root before the
	// shell starts when it has no certificate to serve (ADR-017 §5).
	if len(os.Args) > 1 && os.Args[1] == "certificate" {
		if err := certificate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "verso: certificate: %v\n", err)
			os.Exit(1)
		}
		return
	}
	serve(os.Args[1:])
}

// certificate makes a self-signed pair in dir for the names given that a LAN
// visit can use, and leaves the key's group to the init script.
func certificate(args []string) error {
	if len(args) < 2 || args[0] != "generate" {
		return errors.New("usage: verso certificate generate DIR NAME...")
	}
	names := tlscert.LANNames(args[2:])
	cert, key, err := tlscert.Generate(names, time.Now())
	if err != nil {
		return err
	}
	if err := tlscert.Write(args[1], cert, key); err != nil {
		return err
	}
	fmt.Printf("verso: certificate made for %s\n", strings.Join(names, " "))
	return nil
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

// serve runs the web shell. Its listeners arrive as arguments, one per address,
// because procd restarts a service whose command line changes and not one
// whose environment does: a listener applied from `verso.web` has to restart it.
func serve(args []string) {
	// procd records stdout as info and stderr as err. Keep normal lifecycle
	// messages off the error stream; logd already supplies their timestamp.
	info := log.New(os.Stdout, "", 0)
	flags := flag.NewFlagSet("verso", flag.ContinueOnError)
	var https, plain []string
	flags.Func("listen-https", "an address:port to serve HTTPS on (repeatable)", func(v string) error { https = append(https, v); return nil })
	flags.Func("listen-http", "an address:port to serve HTTP on (repeatable)", func(v string) error { plain = append(plain, v); return nil })
	redirect := flags.String("redirect-https", "", "0 serves the shell over HTTP instead of redirecting to HTTPS")
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	listeners, err := listen.New(https, plain, *redirect)
	if err != nil {
		log.Fatalf("verso: %v", err)
	}
	tlsDir := os.Getenv("VERSO_TLS_DIR")
	if tlsDir == "" {
		tlsDir = "/etc/verso"
	}
	certs := tlscert.NewStore(tlsDir)
	if err := certs.Load(); err != nil {
		log.Fatalf("verso: no certificate to serve HTTPS with: %v", err)
	}

	renderer, err := widget.NewRenderer()
	if err != nil {
		log.Fatalf("verso: %v", err)
	}

	pluginsDir := os.Getenv("VERSO_PLUGINS_DIR")
	if pluginsDir == "" {
		pluginsDir = "/usr/share/verso/plugins"
	}
	// discover reads every plugin manifest, reporting the malformed ones; verb
	// says whether this is the startup scan or a rescan.
	discover := func(verb string) []plugin.Manifest {
		manifests, problems := plugin.Discover(os.DirFS(pluginsDir), "*/manifest.json")
		logProblems(problems)
		info.Printf("verso: %s %d plugin(s) in %s", verb, len(manifests), pluginsDir)
		return manifests
	}
	manifests := discover("discovered")

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
		bundle, problems := i18n.Load(os.DirFS(i18nDir), "*/*.json")
		logProblems(problems)
		logProblems(bundle.LoadPlugins(os.DirFS(pluginsDir), "*/i18n/*.json"))
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
		rescanned := discover("rediscovered")
		srv.SetBundle(loadBundle())
		return rescanned
	})
	srv.SetAllowedHosts(allowedHosts())

	// Both signals are answered from before the first listener is announced, so
	// a signal sent on seeing it is never taken by the default handler.
	// procd stops a service with SIGTERM; answering it is what lets Close run —
	// the background work stopped, and a dev shell's sessions left for the next.
	stopping, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	// The certificate's writer has procd send SIGHUP once a new pair is on disk:
	// it is read again and served from the next handshake, every open
	// connection and session kept. A pair that fails to load leaves the last.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			if err := certs.Load(); err != nil {
				log.Printf("verso: certificate not reloaded, serving the previous one: %v", err)
				continue
			}
			info.Printf("verso: certificate reloaded")
		}
	}()
	endpoints, err := bind(listeners, srv.Handler(), certs, info)
	if err != nil {
		srv.Close()
		log.Fatalf("verso: %v", err)
	}
	err = serveUntil(stopping, endpoints)
	srv.Close()
	if err != nil {
		log.Fatalf("verso: %v", err)
	}
}

// endpoint is one bound listener and the server answering on it; a server
// with a TLS config answers in HTTPS.
type endpoint struct {
	listener net.Listener
	server   *http.Server
}

// bind opens every listener: the shell over HTTPS, then over HTTP either the
// redirect to the first HTTPS listener as bound or, with the redirect off, the
// shell itself. A listener that cannot be bound closes the ones before it.
func bind(c listen.Config, handler http.Handler, certs *tlscert.Store, info *log.Logger) ([]endpoint, error) {
	var bound []endpoint
	open := func(addr, scheme string, server *http.Server) error {
		l, err := listen.Listen(addr)
		if err != nil {
			for _, e := range bound {
				e.listener.Close()
			}
			return err
		}
		info.Printf("verso listening on %s (%s)", l.Addr(), scheme)
		bound = append(bound, endpoint{l, server})
		return nil
	}
	for _, addr := range c.HTTPS {
		secure := &tls.Config{GetCertificate: certs.GetCertificate, MinVersion: tls.VersionTLS12}
		if err := open(addr, "https", &http.Server{Handler: handler, TLSConfig: secure}); err != nil {
			return nil, err
		}
	}
	plain := handler
	if c.Redirect {
		plain = listen.Redirect(bound[0].listener.Addr().String(), certs.Name)
	}
	for _, addr := range c.HTTP {
		if err := open(addr, "http", &http.Server{Handler: plain}); err != nil {
			return nil, err
		}
	}
	return bound, nil
}

// logProblems reports each skipped manifest or catalog; none is fatal.
func logProblems(problems []error) {
	for _, p := range problems {
		log.Printf("verso: %v", p)
	}
}

// serveUntil serves every endpoint until ctx ends or one of them fails, then
// closes every connection at once: a held-open stream would otherwise keep a
// graceful shutdown waiting past procd's patience. A stop asked for is not an
// error.
func serveUntil(ctx context.Context, endpoints []endpoint) error {
	failed := make(chan error, len(endpoints))
	for _, e := range endpoints {
		go func() {
			var err error
			if e.server.TLSConfig != nil {
				err = e.server.ServeTLS(e.listener, "", "")
			} else {
				err = e.server.Serve(e.listener)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				failed <- err
			}
		}()
	}
	var err error
	select {
	case <-ctx.Done():
	case err = <-failed:
	}
	for _, e := range endpoints {
		_ = e.server.Close()
	}
	return err
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

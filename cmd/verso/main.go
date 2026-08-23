// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command verso is the Verso web UI shell for OpenWrt.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/we-are-mono/verso/internal/openwrt"
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

	srv, err := server.New(renderer, openwrt.NewNativeBackend())
	if err != nil {
		log.Fatalf("verso: %v", err)
	}

	log.Printf("verso listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("verso: %v", err)
	}
}

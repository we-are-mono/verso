// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build layoutfixtures

package widget

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteWidgetLayoutFixtures writes the widgets the dev container has no
// page for — a router's panel, which only a board with ports draws — as the
// shell renders them, self-contained (the page inlines the stylesheet), for
// the layout snapshot to record beside the live pages (make layout-snapshot):
// a generated panel with every port state, a board's art, and a board with
// both faces.
func TestWriteWidgetLayoutFixtures(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	dir := filepath.Join(root, "build", "layout-fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile(filepath.Join(root, "internal", "server", "assets", "verso.css"))
	if err != nil {
		t.Fatal(err)
	}
	art, err := os.ReadFile(filepath.Join(root, "profiles", "mono_gateway-dk", "back.svg"))
	if err != nil {
		t.Fatal(err)
	}
	r := newRenderer(t)
	generated := []PortItem{
		{Kind: "rj45", Label: "wan", Iface: "wan", Role: "wan", Linked: true, Speed: "1 Gb/s", Activity: 2},
		{Kind: "rj45", Label: "lan1", Iface: "lan1", Linked: true, Speed: "100 Mb/s", Activity: 1},
		{Kind: "rj45", Label: "lan2", Iface: "lan2"},
		{Kind: "sfp", Label: "sfp1", Iface: "sfp1", Linked: true, Speed: "10 Gb/s", Activity: 4},
	}
	board := []PortItem{
		{Kind: "rj45", Label: "Internet", Iface: "eth0", Role: "wan", Linked: true, Speed: "1 Gb/s", Activity: 2},
		{Kind: "rj45", Label: "Network 1", Iface: "eth1", Linked: true, Speed: "1 Gb/s", Activity: 1},
		{Kind: "rj45", Label: "Network 2", Iface: "eth2"},
		{Kind: "sfp", Label: "Fiber 1", Iface: "eth3", Linked: true, Speed: "10 Gb/s", Activity: 4},
		{Kind: "sfp", Label: "Fiber 2", Iface: "eth4"},
	}
	body := render(t, r, &Ports{Device: "Generated panel", Accent: "emerald", Legend: true, Items: generated}) +
		render(t, r, &Ports{Device: "Board art", Accent: "sky", Legend: true, Items: board, Back: string(art)}) +
		render(t, r, &Ports{Device: "Both faces", Accent: "violet", Items: board, Back: string(art), Front: string(art)})
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><style>` +
		string(css) + `</style></head><body><main style="display:grid;gap:40px;padding:40px">` + body + `</main></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "ports.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderNetMap: the map draws its three tiers — the source and hub as
// icon+label boxes with captions, the leaves as label+detail boxes — plus a
// connector track from the source through the hub to each leaf.
func TestRenderNetMap(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &NetMap{
		Live:   true,
		Source: NetNode{Label: "Internet", Icon: "globe", Detail: "Fibre · 300/40", Variant: "success"},
		Hub:    NetNode{Label: "Router", Icon: "router", Detail: "MyHome"},
		Leaves: []NetNode{{Label: "Wi-Fi", Detail: "8 devices"}, {Label: "Ethernet", Detail: "4 devices"}},
	})
	for _, want := range []string{
		"Internet", "Router", "Wi-Fi", "Ethernet",
		"Fibre · 300/40", "8 devices", "4 devices",
		"border-emerald-300", // the good source node
		"<svg", "verso-netants",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("netmap missing %q:\n%s", want, got)
		}
	}
	// The ants are the only thing on the path — no grey track behind them.
	if strings.Contains(got, "stroke-slate-200") {
		t.Errorf("the path must carry no track behind the ants:\n%s", got)
	}
	// One animated ant line per link when live: source→hub + hub→each leaf = 3.
	if n := strings.Count(got, "verso-netants-live"); n != 3 {
		t.Errorf("want 3 live ant lines, got %d:\n%s", n, got)
	}
}

// TestRenderNetMapStatic: without Live the ants are still drawn (they are the
// path), just not animated.
func TestRenderNetMapStatic(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &NetMap{
		Source: NetNode{Label: "Internet"}, Hub: NetNode{Label: "Router"},
		Leaves: []NetNode{{Label: "Wi-Fi"}},
	})
	if !strings.Contains(got, "verso-netants") {
		t.Errorf("a static map still draws its ant lines:\n%s", got)
	}
	if strings.Contains(got, "verso-netants-live") {
		t.Errorf("a static map must not animate:\n%s", got)
	}
}

// TestDecodeNetMap: the wire shape round-trips through Decode.
func TestDecodeNetMap(t *testing.T) {
	w, err := Decode([]byte(`{"type":"netmap","live":true,
		"source":{"label":"Internet","icon":"globe","variant":"success"},
		"hub":{"label":"Router"},
		"leaves":[{"label":"Wi-Fi","detail":"8 devices"}]}`))
	if err != nil {
		t.Fatalf("decode netmap: %v", err)
	}
	nm, ok := w.(*NetMap)
	if !ok {
		t.Fatalf("decoded %T, want *NetMap", w)
	}
	if nm.Source.Label != "Internet" || nm.Source.Variant != "success" || len(nm.Leaves) != 1 {
		t.Errorf("netmap not decoded: %+v", nm)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
)

// dkInventory is a Mono Gateway DK-shaped inventory — the profiled path the dev
// container can't exercise (the DK isn't in it), so the composition is verified
// against a faithful in-memory inventory instead.
func dkInventory() sensors.Inventory {
	return sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "cluster", Kernel: "cluster-thermal", Source: "thermal zone", Kind: "zone", MilliC: 48000, Warn: 85000, Crit: 95000, HasLimits: true, Level: "nominal", CPU: true},
			{Name: "Memory", Kernel: "ddr-thermal", Source: "thermal zone", Kind: "zone", MilliC: 47000, Warn: 85000, Crit: 95000, HasLimits: true, Level: "nominal", Curated: true, Rank: 1},
			{Name: "temp2", Kernel: "temp2", Source: "tmp431 · 6-4c", Kind: "hwmon", Fault: true},
			{Name: "temperature", Kernel: "temperature", Source: "sfp · xfi0", Kind: "hwmon", MilliC: 47200, Warn: 73000, Crit: 78000, HasLimits: true, Level: "nominal"},
		},
		Powers: []sensors.PowerReading{
			{Name: "USB Power Delivery", Kernel: "power1", Source: "ina234 · 12-40", MilliV: 20100, MilliA: 490, MicroW: 9800000, HasV: true, HasA: true, Main: true},
			{Name: "5V PSU", Kernel: "power1", Source: "ina234 · 12-41", MilliV: 4970, MilliA: 420, MicroW: 2100000, HasV: true, HasA: true},
		},
		Fans: []sensors.FanReading{
			{Name: "System Fan 1", Kernel: "fan1", Source: "emc2305 · 7-2e", RPM: 2622, Duty: 30, HasDuty: true, State: "running", Main: true},
			{Name: "fan2", Kernel: "fan2", Source: "emc2305 · 7-2e", State: "absent"},
		},
		Fibers: []sensors.FiberModule{
			{Cage: "xfi0", Source: "sfp · xfi0", Present: true, TempMilliC: 47200, TempCrit: 78000, VccMilliV: 3300, HasVcc: true, BiasMilliA: 9, HasBias: true, TxMicroW: 542, HasTx: true, RxMicroW: 412, HasRx: true, RxHasLight: true},
			{Cage: "xfi1", Source: "sfp · xfi1", Present: true, TempMilliC: 54600, VccMilliV: 3260, HasVcc: true, BiasMilliA: 20, HasBias: true, TxMicroW: 2150, HasTx: true, RxMicroW: 3, HasRx: true, RxHasLight: false},
		},
	}
}

// The profiled composition: a spinning fan tile in the grid, a Power section that
// itemizes the rails the input feeds (the input itself is the grid tile, not
// repeated), fibre cards with a plain link verdict, and the four grouped readings
// tables — a faulted channel rendering "—".
func TestHardwareProfiledComposition(t *testing.T) {
	s := newServer(t, fakeBackend{})
	inv := dkInventory()
	profile := &sensors.Profile{Name: "Mono Gateway Development Kit"}

	sections := []widget.Widget{
		hardwareVitals(identityTranslator, inv), hardwareTemps(identityTranslator, profile, inv),
		hardwarePower(identityTranslator, inv), hardwareFans(identityTranslator, inv), hardwareFibre(inv),
	}
	sections = append(sections, hardwareReadings(inv)...)
	var live []widget.Widget
	for _, w := range sections {
		if w != nil {
			live = append(live, w)
		}
	}
	var buf strings.Builder
	if err := s.widgets.RenderWithToken(&buf, &widget.Stack{Children: live}, "", "en", nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	body := buf.String()

	for _, want := range []string{
		"Cooling", "2,622", // the fan leads the grid with the spinning glyph
		"Power", "5V PSU", "4.97 V", "2.1 W", // the Power section itemizes the fed rails
		"SFP", "xfi0", "xfi1", "Receiving light", "No signal", "dBm", // fibre cards
		"cluster-thermal", "USB Power Delivery", "sensor fault", // the full readings tables
	} {
		if !strings.Contains(body, want) {
			t.Errorf("profiled composition missing %q", want)
		}
	}
	// The input rail leads the grid, so the Power section does not repeat it — but
	// the full readings table still lists it (the value above appears once, always
	// in the table): once in the grid, once in the readings table.
	if strings.Count(body, "USB Power Delivery") < 2 {
		t.Errorf("the input rail should appear in both the grid and the readings table")
	}
	// One connected fan means no Fans tile-strip section — the cooling glyph turns
	// exactly once, in the grid. (The readings Fans table lists both channels but
	// draws no glyph.)
	if n := strings.Count(body, "verso-glyph-fan"); n != 1 {
		t.Errorf("the fan glyph should appear once (the grid tile), got %d", n)
	}
}

// The DK's rear-panel art is embedded in the binary and loads as trusted content.
func TestHardwarePanelArtEmbedded(t *testing.T) {
	svg := readPanel("mono_gateway-dk", "back.svg")
	if !strings.Contains(svg, `data-verso-port="eth0"`) || !strings.Contains(svg, "led-link") {
		t.Fatalf("DK back.svg should embed and carry its live layer, got %d bytes", len(svg))
	}
	if readPanel("mono_gateway-dk", "front.svg") != "" {
		t.Error("the DK ships no front.svg; readPanel should return empty, not error")
	}
}

// The Hardware page names the box in its serif display masthead (no eyebrow), and
// appears as the first System tab, active on its own page.
func TestHardwarePageMastheadAndTab(t *testing.T) {
	srv := newServer(t, fakeBackend{board: openwrt.Board{Model: "Supermicro H13SAE-MF"}})
	body := get(t, srv, "/system/hardware").Body.String()
	for _, want := range []string{
		"Supermicro H13SAE-MF",                           // the model is the headline
		"verso-page-heading",                             // rendered as the serif display masthead (no kicker)
		`href="/system/hardware"`, `aria-current="page"`, // the active System tab
		"All sensors", // the full instrument panel
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Hardware page missing %q", want)
		}
	}
	// Hardware leads the shell's System tabs, before Access.
	if i, j := strings.Index(body, `href="/system/hardware"`), strings.Index(body, `href="/system/access"`); i < 0 || j < 0 || i > j {
		t.Errorf("Hardware tab should precede Access (hardware=%d access=%d)", i, j)
	}
}

// The sidebar's "This device" row is the doorway to the Hardware page — it names
// the device, so it belongs here, not to Maintenance.
func TestDeviceRowLeadsToHardware(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	// The home page carries no System tabs, so the only Hardware link on it is the
	// footer device row.
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `href="/system/hardware"`) {
		t.Errorf("the sidebar device row should link to the Hardware page:\n%s", firstLines(body, 0))
	}
}

func firstLines(s string, _ int) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

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
			{Cage: "xfi0", Source: "sfp · xfi0", TempMilliC: 47200, VccMilliV: 3300, HasVcc: true, BiasMilliA: 9, HasBias: true, TxMicroW: 542, HasTx: true, RxMicroW: 412, HasRx: true, RxHasLight: true},
			{Cage: "xfi1", Source: "sfp · xfi1", TempMilliC: 54600, VccMilliV: 3260, HasVcc: true, BiasMilliA: 20, HasBias: true, TxMicroW: 2150, HasTx: true, RxMicroW: 3, HasRx: true, RxHasLight: false},
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
		hardwareVitals(identityTranslator, profile, inv), hardwareTemps(identityTranslator, profile, inv),
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

// TestReadingsKeepTheirCellsApart: a reading's value and its limits are
// neighbouring columns, the value set right and the limits left, so each cell
// keeps its own inset — a four-column table has the measure for it — or the
// two run together ("46.9 °Chigh 81 °C").
func TestReadingsKeepTheirCellsApart(t *testing.T) {
	s := newServer(t, fakeBackend{})
	var buf strings.Builder
	if err := s.widgets.RenderWithToken(&buf, &widget.Stack{Children: hardwareReadings(dkInventory())}, "", "en", nil); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.Contains(body, "px-3.5 pt-2.5 pb-2.25 leading-5") {
		t.Errorf("the readings' cells give up their inset:\n%s", body)
	}
}

// unprofiledInventory is the dev container's box: no profile, two drives that
// both call their reading "Composite", a GPU edge with no limits, a drive's
// extra sensor whose limit was a sentinel, and no fans or main power rail.
func unprofiledInventory() sensors.Inventory {
	return sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "Composite", Kernel: "Composite", Source: "nvme · 05:00", Kind: "hwmon", MilliC: 46900, Warn: 81000, Crit: 85000, HasLimits: true, Level: "nominal", Curated: true},
			{Name: "Composite", Kernel: "Composite", Source: "nvme · 0e:00", Kind: "hwmon", MilliC: 44900, Warn: 81000, Crit: 85000, HasLimits: true, Level: "nominal", Curated: true, Rank: 1},
			{Name: "edge", Kernel: "edge", Source: "amdgpu", Kind: "hwmon", MilliC: 71000, Curated: true, Rank: 2},
			{Name: "Sensor 1", Kernel: "Sensor 1", Source: "nvme · 05:00", Kind: "hwmon", MilliC: 46900, SentinelDiscarded: true},
		},
	}
}

// TestAnUnprofiledBoxSaysOnlyWhatItHas: a box with no profile shows what the
// kernel reports and says nothing about what it lacks — no notice that it has
// no fans, no note restating that bars need limits, no driver jargon, and no
// line of bus addresses under a reading's name.
func TestAnUnprofiledBoxSaysOnlyWhatItHas(t *testing.T) {
	s := newServer(t, fakeBackend{})
	inv := unprofiledInventory()
	live := append([]widget.Widget{hardwareTemps(identityTranslator, nil, inv)}, hardwareReadings(inv)...)
	var buf strings.Builder
	if err := s.widgets.RenderWithToken(&buf, &widget.Stack{Children: live}, "", "en", nil); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, never := range []string{"No fans or curated power rails", "bars drawn only where", "sentinel"} {
		if strings.Contains(body, never) {
			t.Errorf("the page says %q", never)
		}
	}
	temps := body[strings.Index(body, "Temperatures"):strings.Index(body, "All sensors")]
	if strings.Contains(temps, "nvme · 05:00") {
		t.Errorf("Temperatures prints a reading's bus address under its name:\n%s", temps)
	}
	// The readings are rows, each with its own air above and below its
	// hairline; a spaced stack would add a gap under each hairline that the
	// row's other side does not have.
	if strings.Contains(temps, "verso-stack space-y-") || !strings.Contains(temps, "data-verso-rows") {
		t.Errorf("Temperatures spaces its rows apart instead of letting each keep its own air:\n%s", temps)
	}
}

// TestOnlyAProfileEarnsTheVitals: the vitals across the top are a profile's
// picks — the processor, the input rail, the main fan. A box no profile
// describes has no such picks, only guesses, so it draws no vitals; its
// readings stand in Temperatures and All sensors.
func TestOnlyAProfileEarnsTheVitals(t *testing.T) {
	if grid := hardwareVitals(identityTranslator, nil, unprofiledInventory()); grid != nil {
		t.Errorf("an unprofiled box draws vitals: %#v", grid)
	}
	if grid := hardwareVitals(identityTranslator, &sensors.Profile{Name: "Mono Gateway Development Kit"}, dkInventory()); grid == nil {
		t.Error("a profiled box draws no vitals")
	}
}

// TestAFibreModuleIsAnArtifactCard: an SFP module is a thing the box holds,
// read as one, so it wears the card Access gives its HTTPS certificate — the
// sand frame (a pixel out onto the grid's lines), the title over its line, the
// facts set left under it.
func TestAFibreModuleIsAnArtifactCard(t *testing.T) {
	s := newServer(t, fakeBackend{})
	inv := dkInventory()
	var buf strings.Builder
	if err := s.widgets.RenderWithToken(&buf, fibreCard(&inv.Fibers[0]), "", "en", nil); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	for _, want := range []string{
		`<section class="-mt-px -ml-px w-[calc(100%_+_1px)] rounded-xs border border-rule bg-quiet px-5 pb-4.75">`,
		`<dt class="shrink-0 text-meta sm:w-24">Link</dt>`,
		`<p class="font-mono text-base leading-5 font-medium text-body">xfi0</p>`, // the cage, as the machine names it
	} {
		if !strings.Contains(body, want) {
			t.Errorf("an SFP module is not the certificate's card; missing %s:\n%s", want, body)
		}
	}
	// The section says nothing about where its readings come from: a module's
	// facts are its own, and a note beside the heading restates it.
	if section := hardwareFibre(inv).(*widget.Section); section.Meta != "" {
		t.Errorf("Fiber modules carries a note: %q", section.Meta)
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

// The Hardware page wears every page's masthead — the sand bar, its title
// the page's name — and appears as the first System tab, active on its own
// page. The box's own name is the top bar's, so the title does not repeat it.
func TestHardwarePageMastheadAndTab(t *testing.T) {
	srv := newServer(t, fakeBackend{board: openwrt.Board{Model: "Supermicro H13SAE-MF"}})
	body := get(t, srv, "/system/hardware").Body.String()
	for _, want := range []string{
		`<div data-verso-masthead class="mb-5 py-4">`,    // every page's sand bar
		`<h1 class="verso-page-heading">Hardware</h1>`,   // titled as the page
		`href="/system/hardware"`, `aria-current="page"`, // the active System tab
		"All sensors", // the full instrument panel
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Hardware page missing %q", want)
		}
	}
	if strings.Contains(body, `verso-page-heading">Supermicro H13SAE-MF`) {
		t.Error("the page's title repeats the box's name the top bar already carries")
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/machine"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// dkInventory is a Mono Gateway DK-shaped inventory as ResolveAll gives it on
// the DK — the profiled path the dev container can't exercise (the DK isn't in
// it), so the page is verified against a faithful in-memory inventory instead.
// The unconnected remote diode is a reading the profile does not name, and the
// SFP modules' temperatures are their own.
func dkInventory() sensors.Inventory {
	return sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "cluster", Kernel: "cluster-thermal", Source: "thermal zone", Kind: "zone", MilliC: 48000, Warn: 85000, Crit: 95000, HasLimits: true, Level: "nominal", CPU: true, Named: true, Series: "temp.cluster-thermal"},
			{Name: "Memory", Kernel: "ddr-thermal", Source: "thermal zone", Kind: "zone", MilliC: 47000, Warn: 85000, Crit: 95000, HasLimits: true, Level: "nominal", Curated: true, Named: true, Rank: 1, Series: "temp.ddr-thermal"},
			{Name: "PCB near CPU", Kernel: "temp1", Source: "tmp431 · 13-4c", Kind: "hwmon", MilliC: 41200, Warn: 85000, Crit: 108000, HasLimits: true, Level: "nominal", Curated: true, Named: true, Rank: 5, Series: "temp.temp1"},
			{Name: "temp2", Kernel: "temp2", Source: "tmp431 · 14-4c", Kind: "hwmon", Fault: true, Series: "temp.temp2"},
			{Name: "temperature", Kernel: "temperature", Source: "sfp · xfi0", Kind: "hwmon", MilliC: 47200, Warn: 73000, Crit: 78000, HasLimits: true, Level: "nominal", Fiber: true, Series: "temp.temperature.sfp · xfi0"},
			{Name: "temperature", Kernel: "temperature", Source: "sfp · xfi1", Kind: "hwmon", MilliC: 54600, Warn: 90000, Crit: 95000, HasLimits: true, Level: "nominal", Fiber: true, Series: "temp.temperature.sfp · xfi1"},
		},
		Powers: []sensors.PowerReading{
			{Name: "5V PSU", Kernel: "power1", Source: "ina234 · 12-41", MilliV: 4970, MilliA: 420, MicroW: 2100000, HasV: true, HasA: true, Named: true},
			{Name: "USB Power Delivery", Kernel: "power1", Source: "ina234 · 12-40", MilliV: 20100, MilliA: 490, MicroW: 9800000, HasV: true, HasA: true, Main: true, Named: true},
			{Name: "1.2V DDR PSU", Kernel: "power1", Source: "ina234 · 12-43", MilliV: 1200, MilliA: 710, MicroW: 852000, HasV: true, HasA: true, Named: true},
		},
		Fans: []sensors.FanReading{
			{Name: "System Fan 1", Kernel: "fan1", Source: "emc2305 · 15-2e", RPM: 2622, Duty: 30, HasDuty: true, State: "running", Main: true, Named: true},
			{Name: "System Fan 2", Kernel: "fan2", Source: "emc2305 · 15-2e", State: "absent", Named: true},
		},
		Fibers: []sensors.FiberModule{
			{Cage: "xfi0", Source: "sfp · xfi0", TempMilliC: 47200, VccMilliV: 3300, HasVcc: true, BiasMilliA: 9, HasBias: true, TxMicroW: 542, HasTx: true, RxMicroW: 412, HasRx: true, RxHasLight: true},
			{Cage: "xfi1", Source: "sfp · xfi1", TempMilliC: 54600, VccMilliV: 3260, HasVcc: true, BiasMilliA: 20, HasBias: true, TxMicroW: 2150, HasTx: true, RxMicroW: 3, HasRx: true, RxHasLight: false},
		},
	}
}

// unprofiledInventory is the dev container's box: no profile, two drives that
// both call their reading "Composite", a GPU edge with no limits, a drive's
// extra sensor whose limit was a sentinel, a faulted channel, a GPU power
// channel, and no fans or fibre.
func unprofiledInventory() sensors.Inventory {
	return sensors.Inventory{
		Temps: []sensors.TempReading{
			{Name: "Tctl", Kernel: "Tctl", Source: "k10temp", Kind: "hwmon", MilliC: 77900, CPU: true},
			{Name: "Composite", Kernel: "Composite", Source: "nvme · 05:00", Kind: "hwmon", MilliC: 46900, Warn: 81000, Crit: 85000, HasLimits: true, Level: "nominal", Curated: true},
			{Name: "Composite", Kernel: "Composite", Source: "nvme · 0e:00", Kind: "hwmon", MilliC: 44900, Warn: 81000, Crit: 85000, HasLimits: true, Level: "nominal", Curated: true, Rank: 1},
			{Name: "edge", Kernel: "edge", Source: "amdgpu", Kind: "hwmon", MilliC: 71000, Curated: true, Rank: 2},
			{Name: "Sensor 1", Kernel: "Sensor 1", Source: "nvme · 05:00", Kind: "hwmon", MilliC: 46900, SentinelDiscarded: true},
			{Name: "temp3", Kernel: "temp3", Source: "nct6798", Kind: "hwmon", Fault: true},
		},
		Powers: []sensors.PowerReading{
			{Name: "PPT", Kernel: "PPT", Source: "amdgpu", MicroW: 7000},
		},
	}
}

func dkInput() hardwareInput {
	return hardwareInput{
		board:   openwrt.Board{Model: "Mono Gateway DK", BoardName: "mono,gateway-dk", System: "ARMv8 Processor rev 2"},
		profile: &sensors.Profile{Name: "Mono Gateway Development Kit", Processor: "NXP LS1046A"},
		inv:     dkInventory(),
		facts:   machine.Facts{Cores: 4, MaxKHz: 1600000, MemoryBytes: 8070316 * 1024, Disk: &machine.Disk{Bytes: 61071360 * 512, Kind: "eMMC", Wear: &machine.Wear{Band: 1, PreEOL: 1}}, Flash: &machine.Flash{Part: "n25q512a", Bytes: 64 << 20}, Root: "/dev/mmcblk0p4"},
		hw:      openwrt.HardwareState{Clock: &openwrt.Clock{Part: "pcf2131", Battery: "good"}, Boot: dkBootEnv()},
		photo:   &boardPhoto{href: "/assets/boards/0123456789ab.png", width: 800, height: 592},
	}
}

// dkBootEnv is the DK's U-Boot slot variables, as zamanifi prints them.
func dkBootEnv() map[string]string {
	return map[string]string{
		"slot": "b", "bootcount": "0", "bootlimit": "3", "upgrade_available": "0",
		"set_slot_a": "setenv bootpart 1; setenv rootpart 2",
		"set_slot_b": "setenv bootpart 3; setenv rootpart 4",
	}
}

func unprofiledInput() hardwareInput {
	return hardwareInput{
		board: openwrt.Board{Model: "Supermicro H13SAE-MF", BoardName: "supermicro-h13sae-mf", System: "AMD Ryzen 7 7700 8-Core Processor"},
		inv:   unprofiledInventory(),
		facts: machine.Facts{Cores: 16, MemoryBytes: 32 << 30},
	}
}

func renderCards(t *testing.T, in hardwareInput) string {
	t.Helper()
	s := newServer(t, fakeBackend{})
	var buf strings.Builder
	if err := s.widgets.RenderWithToken(&buf, &widget.Stack{Children: hardwareCards(identityTranslator, in)}, "", "en", nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

var cardTitle = regexp.MustCompile(`<h2 class="[^"]*">([^<]*)</h2>`)

func cardTitles(body string) []string {
	var out []string
	for _, m := range cardTitle.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// card is the markup of the card titled title, up to the next card.
func card(t *testing.T, body, title string) string {
	t.Helper()
	marker := ">" + title + "</h2>"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no %q card:\n%s", title, body)
	}
	rest := body[i+len(marker):]
	if j := strings.Index(rest, `<section class="verso-card"`); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// The page is the board card, then one card per kind of reading, in order,
// and one per fiber module.
func TestHardwareCardsStandInOrder(t *testing.T) {
	got := cardTitles(renderCards(t, dkInput()))
	want := []string{"Mono Gateway Development Kit", "Temperatures", "Power", "Cooling", "SFP&#43; module", "SFP&#43; module"}
	if !slices.Equal(got, want) {
		t.Fatalf("cards %q, want %q", got, want)
	}
}

// A board with no reading of a kind has no card for it.
func TestAKindWithNoReadingHasNoCard(t *testing.T) {
	got := cardTitles(renderCards(t, unprofiledInput()))
	want := []string{"Supermicro H13SAE-MF", "Temperatures", "Power"}
	if !slices.Equal(got, want) {
		t.Fatalf("cards %q, want %q", got, want)
	}
}

// The page shows each reading once: no tile strip over the cards, and no
// table of all sensors under them.
func TestEachReadingAppearsOnce(t *testing.T) {
	body := renderCards(t, dkInput())
	for _, once := range []string{">cluster<", ">USB Power Delivery<", ">System Fan 1<"} {
		if n := strings.Count(body, once); n != 1 {
			t.Errorf("%s appears %d times, want once", once, n)
		}
	}
	for _, never := range []string{"All sensors", "divide-x divide-rule border-y", "Processor</span>", "Power draw"} {
		if strings.Contains(body, never) {
			t.Errorf("the page still draws %q", never)
		}
	}
}

// The board card is titled with the profile's name, else the model, else
// "Board", so the card never stands under an empty heading.
func TestTheBoardCardsTitle(t *testing.T) {
	if got := cardTitles(renderCards(t, dkInput()))[0]; got != "Mono Gateway Development Kit" {
		t.Errorf("profiled title %q", got)
	}
	if got := cardTitles(renderCards(t, unprofiledInput()))[0]; got != "Supermicro H13SAE-MF" {
		t.Errorf("unprofiled title %q", got)
	}
	in := unprofiledInput()
	in.board = openwrt.Board{}
	if got := cardTitles(renderCards(t, in))[0]; got != "Board" {
		t.Errorf("a board that says nothing of itself is titled %q", got)
	}
}

// The board card says what the box is made of, a fact it cannot answer left
// out.
func TestTheBoardCardsFacts(t *testing.T) {
	board := card(t, renderCards(t, dkInput()), "Mono Gateway Development Kit")
	for _, want := range []string{
		// the board's photograph, named by the board
		`<img class="verso-figure__image" src="/assets/boards/0123456789ab.png" alt="Mono Gateway Development Kit" width="800" height="592"`,
		">NXP LS1046A · 4 cores · 1.6\U0000202fGHz<", // the profile's chip, the kernel's cores and clock
		">7.7\U0000202fGiB<",
		">29\U0000202fGiB eMMC · 0–10\U0000202f% worn<", // whole GiB from ten up, then the eMMC's own wear estimate
		">n25q512a · 64\U0000202fMiB<",                  // the part as the kernel names it, then its size
		">B<",                                           // the slot the kernel booted
		">pcf2131 · battery good<",                      // the clock the system time was set from
		">mono,gateway-dk<",
	} {
		if !strings.Contains(board, want) {
			t.Errorf("board card missing %q:\n%s", want, board)
		}
	}
	// The facts stand at the left, the photograph beside them on the right;
	// the router's panel is the Overview's, not the board card's.
	figure, list, photo := strings.Index(board, `<div class="verso-grid" data-verso-style="figure"`), strings.Index(board, "<dl"), strings.Index(board, "verso-figure")
	if figure < 0 || list < figure || photo < list {
		t.Errorf("the facts should stand beside the photograph, the facts first:\n%s", board)
	}
	if strings.Contains(board, "verso-ports") {
		t.Errorf("the board card draws the router's panel:\n%s", board)
	}
	// One list, read down: Processor, Memory, Storage, Boot slot, NOR flash,
	// Clock, Board ID.
	if n := strings.Count(board, "<dl"); n != 1 || strings.Contains(board, "data-verso-facts") {
		t.Errorf("the board's facts are %d lists, want one:\n%s", n, board)
	}
	last := -1
	for _, label := range []string{">Processor<", ">Memory<", ">Storage<", ">Boot slot<", ">NOR flash<", ">Clock<", ">Board ID<"} {
		i := strings.Index(board, label)
		if i < last {
			t.Errorf("%s out of order:\n%s", label, board)
		}
		last = i
	}

	board = card(t, renderCards(t, unprofiledInput()), "Supermicro H13SAE-MF")
	if !strings.Contains(board, ">AMD Ryzen 7 7700 8-Core Processor · 16 cores<") {
		t.Errorf("an unprofiled processor reads as the kernel names it, with its cores:\n%s", board)
	}
	if strings.Contains(board, "Storage") || strings.Contains(board, "GHz") || strings.Contains(board, "NOR flash") ||
		strings.Contains(board, "Boot slot") || strings.Contains(board, "Clock") {
		t.Errorf("a fact the box cannot answer should be left out:\n%s", board)
	}

	// A chip the SPI NOR framework names none says its size alone.
	in := dkInput()
	in.facts.Flash = &machine.Flash{Bytes: 8 << 20}
	if board := card(t, renderCards(t, in), "Mono Gateway Development Kit"); !strings.Contains(board, ">8\U0000202fMiB<") {
		t.Errorf("an unnamed NOR should read its size alone:\n%s", board)
	}

	// A worn eMMC marks its row in the tone the heading speaks it in; one
	// past its rated life says so rather than a band.
	in = dkInput()
	in.facts.Disk.Wear = &machine.Wear{Band: 11, PreEOL: 3}
	board = card(t, renderCards(t, in), "Mono Gateway Development Kit")
	if !strings.Contains(board, ">29\U0000202fGiB eMMC · past its rated wear<") || !strings.Contains(board, `data-verso-tone="danger"`) {
		t.Errorf("a worn-out eMMC should say so, marked in danger:\n%s", board)
	}
}

// The boot slot says which slot runs, the slot U-Boot boots next when a
// switch is staged, and how far a new image's trial has come; the clock says
// a low battery in the warning tone.
func TestTheBootSlotAndClock(t *testing.T) {
	cases := []struct {
		name string
		edit func(*hardwareInput)
		want string
	}{
		{"a staged switch", func(in *hardwareInput) { in.hw.Boot["slot"] = "a" }, ">B · A on next boot<"},
		{"on trial", func(in *hardwareInput) { in.hw.Boot["upgrade_available"], in.hw.Boot["bootcount"] = "1", "2" }, ">B · trial boot 2 of 3<"},
		{"a low battery", func(in *hardwareInput) { in.hw.Clock.Battery = "low" }, ">pcf2131 · battery low<"},
		{"a clock that does not say", func(in *hardwareInput) { in.hw.Clock.Battery = "" }, ">pcf2131<"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := dkInput()
			c.edit(&in)
			if board := card(t, renderCards(t, in), "Mono Gateway Development Kit"); !strings.Contains(board, c.want) {
				t.Errorf("board card missing %q:\n%s", c.want, board)
			}
		})
	}
	in := dkInput()
	in.hw.Clock.Battery = "low"
	if board := card(t, renderCards(t, in), "Mono Gateway Development Kit"); !strings.Contains(board, `data-verso-tone="warning"`) {
		t.Errorf("a low clock battery should mark its row in the warning tone:\n%s", board)
	}
}

// Temperatures is one table: the CPU first, the profile's entries in its
// order, nothing the profile leaves out, and an SFP module's temperature on
// its fibre list.
func TestTemperaturesListTheProfilesReadings(t *testing.T) {
	temps := card(t, renderCards(t, dkInput()), "Temperatures")
	for _, col := range []string{">Reading<", ">Source<", ">Temperature<", ">Limits<"} {
		if !strings.Contains(temps, col) {
			t.Errorf("Temperatures missing the column %s", col)
		}
	}
	i, j, k := strings.Index(temps, ">cluster<"), strings.Index(temps, ">Memory<"), strings.Index(temps, ">PCB near CPU<")
	if i < 0 || j < i || k < j {
		t.Errorf("rows out of order: cluster=%d Memory=%d PCB=%d", i, j, k)
	}
	for _, never := range []string{"temp2", "sfp · xfi0"} {
		if strings.Contains(temps, never) {
			t.Errorf("Temperatures lists %q, which the profile does not name", never)
		}
	}
	// A zone's source is its type; a chip's is the chip and its channel.
	for _, src := range []string{">cluster-thermal<", ">tmp431 · 13-4c temp1<"} {
		if !strings.Contains(temps, src) {
			t.Errorf("Temperatures missing the source %s", src)
		}
	}
}

// TestTemperaturesChartOneReadingAtATime: over its table, the card asks which
// reading to chart and over which window, as a sentence ("Show chart for [CPU]
// over [Last 24 hours]"): the table's rows in order, then each SFP module, by
// the series their history is under; the CPU and the day chosen. The graph
// stands under the field and over the table, and the card's heading carries no
// control.
func TestTemperaturesChartOneReadingAtATime(t *testing.T) {
	in := dkInput()
	in.tempGraph = &widget.Chart{ID: temperatureGraphID, Span: 24 * time.Hour, Series: []widget.ChartSeries{{Values: []float64{48, 49}}}}
	temps := card(t, renderCards(t, in), "Temperatures")
	for _, want := range []string{
		">Show chart for<", `class="verso-joined__word">over</span>`,
		`hx-get="/system/hardware/temperature"`, `hx-target="#hardware-temperature-graph"`,
		`<option value="temp.cluster-thermal" selected>cluster</option>`,
		`<option value="temp.temperature.sfp · xfi0">SFP&#43; module xfi0</option>`,
		`<option value="temp.temperature.sfp · xfi1">SFP&#43; module xfi1</option>`,
		`<option value="day" selected>Last 24 hours</option>`, `<option value="year">Last year</option>`,
	} {
		if !strings.Contains(temps, want) {
			t.Errorf("Temperatures missing %q:\n%s", want, temps)
		}
	}
	// The choices follow the table's rows, then the modules; a faulted
	// channel the profile leaves out is no choice.
	order := []string{">cluster</option>", ">Memory</option>", ">PCB near CPU</option>", ">SFP&#43; module xfi0</option>", ">SFP&#43; module xfi1</option>"}
	for i := 1; i < len(order); i++ {
		if strings.Index(temps, order[i-1]) > strings.Index(temps, order[i]) {
			t.Errorf("%s comes after %s", order[i-1], order[i])
		}
	}
	if strings.Contains(temps, ">temp2</option>") {
		t.Error("a channel the profile does not name is a choice")
	}
	field, graph, table := strings.Index(temps, ">Show chart for<"), strings.Index(temps, `id="hardware-temperature-graph"`), strings.Index(temps, "<table")
	if field < 0 || graph < field || table < graph {
		t.Errorf("not field, graph, table: field=%d graph=%d table=%d", field, graph, table)
	}
	if strings.Index(temps, "<select") < strings.Index(temps, "bar runs") {
		t.Error("the card's heading carries a control")
	}
}

// Each row's bar runs to its protect limit, the blocks past its warn limit
// shaded, and its Limits say them.
func TestATemperatureShowsItsHeadroom(t *testing.T) {
	temps := card(t, renderCards(t, dkInput()), "Temperatures")
	for _, want := range []string{
		// 48 of 95, warning at 85 of 95; the protect limit is the track's end
		`<span class="verso-meter__track" style="--verso-meter-fill: 50; --verso-meter-warn: 89">`,
		">48.0\U0000202f°C<",
		// the limits in words, their figures a step heavier
		`>warns <span class="verso-meter__number">85` + "\U0000202f" + `°C</span> · protects <span class="verso-meter__number">95` + "\U0000202f" + `°C</span><`,
	} {
		if !strings.Contains(temps, want) {
			t.Errorf("Temperatures missing %q", want)
		}
	}
	if strings.Contains(temps, "verso-meter__tick") {
		t.Error("a temperature ticks its limits; the warn zone says them")
	}
	// The warn zone is the range's Strong Hairline, past the Rule of the rest.
	css, err := os.ReadFile(filepath.Join("assets", "blocks", "meter.css"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "background-image: linear-gradient(to right, var(--color-rule) calc(var(--verso-meter-zone) * var(--verso-meter-step)), var(--color-rule-strong) 0);"; !strings.Contains(string(css), want) {
		t.Errorf("blocks/meter.css does not shade the warn zone:\nwant %s", want)
	}
}

// Every temperature on a box with no profile, under the kernel's names; one
// with no limits has no bar, and a faulted one reads "—".
func TestAnUnprofiledBoxListsEveryTemperature(t *testing.T) {
	temps := card(t, renderCards(t, unprofiledInput()), "Temperatures")
	for _, want := range []string{">Tctl<", ">edge<", ">Sensor 1<", ">temp3<", ">no limits reported<", ">sensor fault<", ">71.0\U0000202f°C<"} {
		if !strings.Contains(temps, want) {
			t.Errorf("Temperatures missing %q", want)
		}
	}
	if n := strings.Count(temps, ">Composite<"); n != 2 {
		t.Errorf("both drives' Composite should list, got %d", n)
	}
	// A chip's channel that is named for itself is not said twice.
	if strings.Contains(temps, "nvme · 05:00 Composite") {
		t.Errorf("a labelled channel repeats its name in its source:\n%s", temps)
	}
}

// Power is every rail, the input first and tagged, the rest by draw, with no
// total beside the title: the input's row says it.
func TestPowerLeadsWithTheInput(t *testing.T) {
	power := card(t, renderCards(t, dkInput()), "Power")
	i, j, k := strings.Index(power, ">USB Power Delivery<"), strings.Index(power, ">5V PSU<"), strings.Index(power, ">1.2V DDR PSU<")
	if i < 0 || j < i || k < j {
		t.Errorf("rails out of order: input=%d 5V=%d DDR=%d", i, j, k)
	}
	for _, want := range []string{">input<", ">20.10\U0000202fV<", ">0.49\U0000202fA<", ">9.8\U0000202fW<", ">852\U0000202fmW<", ">ina234 · 12-40<"} {
		if !strings.Contains(power, want) {
			t.Errorf("Power missing %q", want)
		}
	}
	if strings.Contains(power, "data-verso-section-meta") || strings.Contains(power, "rails") {
		t.Errorf("Power's heading carries a summary:\n%s", power)
	}
	if strings.Contains(card(t, renderCards(t, unprofiledInput()), "Power"), ">input<") {
		t.Error("an unprofiled rail is tagged the input")
	}
}

// Cooling stands for one fan as for several, each fan's state last.
func TestCoolingListsEveryFan(t *testing.T) {
	cooling := card(t, renderCards(t, dkInput()), "Cooling")
	for _, want := range []string{">System Fan 1<", ">2,622\U0000202frpm<", ">30\U0000202f%<", ">Running<", ">System Fan 2<", ">Not connected<", ">emc2305 · 15-2e fan1<"} {
		if !strings.Contains(cooling, want) {
			t.Errorf("Cooling missing %q", want)
		}
	}
	in := dkInput()
	in.inv.Fans = in.inv.Fans[:1]
	in.inv.Fans[0].State, in.inv.Fans[0].RPM = "fault", 0
	cooling = card(t, renderCards(t, in), "Cooling")
	if !strings.Contains(cooling, ">Stalled<") || !strings.Contains(cooling, `data-verso-tone="warning"`) {
		t.Errorf("a stalled fan should read Stalled in the warning tone:\n%s", cooling)
	}
}

// Each SFP module is a card of its own, titled "SFP+ module" with its cage's
// chip in the header, its readings one list under it; the modules' cards
// stand side by side.
func TestEachModuleIsACard(t *testing.T) {
	page := renderCards(t, dkInput())
	row := strings.Index(page, `<div class="verso-grid" data-verso-style="cards"`)
	if row < 0 || row > strings.Index(page, ">SFP&#43; module</h2>") {
		t.Errorf("the modules' cards should stand in one row:\n%s", page)
	}
	var modules []string
	for _, s := range strings.Split(page, `<section class="verso-card"`) {
		if strings.Contains(s, ">SFP&#43; module</h2>") {
			modules = append(modules, s)
		}
	}
	if len(modules) != 2 {
		t.Fatalf("%d module cards, want one per module", len(modules))
	}
	for i, want := range [][]string{{">xfi0</span>", ">Receiving light<"}, {">xfi1</span>", ">No signal<"}} {
		for _, w := range want {
			if !strings.Contains(modules[i], w) {
				t.Errorf("module card %d missing %q:\n%s", i, w, modules[i])
			}
		}
		if strings.Contains(modules[i], "data-verso-properties-head") {
			t.Errorf("the card's header names the module; its list has no head of its own:\n%s", modules[i])
		}
	}
}

// A module's readings are figures with their units, set in the sans as every
// other figure is; mono is for strings someone could type back in.
func TestAModulesReadingsAreFigures(t *testing.T) {
	facts := fibreFacts(&dkInventory().Fibers[0])
	for _, item := range facts.(*widget.Properties).Items {
		if item.Mono {
			t.Errorf("%s is a figure, not a machine string: %+v", item.Label, item)
		}
	}
}

// The heading reads "Hardware" while all is well, and says what needs
// attention in its tone, in the Overview card's words.
func TestTheHeadingSaysWhatNeedsAttention(t *testing.T) {
	cases := []struct {
		name          string
		edit          func(*sensors.Inventory)
		wear          machine.Wear
		heading, tone string
	}{
		{"all well", func(*sensors.Inventory) {}, machine.Wear{Band: 1, PreEOL: 1}, "Hardware", "neutral"},
		{"a hot CPU", func(inv *sensors.Inventory) { inv.Temps[0].MilliC, inv.Temps[0].Level = 87000, "warn" }, machine.Wear{}, "cluster is hot", "warning"},
		{"a critical CPU beside a stopped fan", func(inv *sensors.Inventory) {
			inv.Temps[0].MilliC, inv.Temps[0].Level = 96000, "critical"
			inv.Fans[0].State, inv.Fans[0].RPM = "fault", 0
		}, machine.Wear{}, "cluster is hot", "danger"},
		{"a stopped fan", func(inv *sensors.Inventory) { inv.Fans[0].State, inv.Fans[0].RPM = "fault", 0 }, machine.Wear{}, "System Fan 1 stopped", "warning"},
		{"a wearing eMMC", func(*sensors.Inventory) {}, machine.Wear{Band: 9, PreEOL: 1}, "Storage is wearing out", "warning"},
		{"a hot CPU beside a worn-out eMMC", func(inv *sensors.Inventory) { inv.Temps[0].MilliC, inv.Temps[0].Level = 87000, "warn" },
			machine.Wear{Band: 11, PreEOL: 3}, "Storage is worn out", "danger"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := dkInventory()
			c.edit(&inv)
			facts := machine.Facts{Disk: &machine.Disk{Bytes: 1 << 30, Kind: "eMMC", Wear: &c.wear}}
			if h := hardwareHeading(identityTranslator, inv, hardwareAlerts(facts, openwrt.HardwareState{})); h.Heading != c.heading || h.Tone != c.tone {
				t.Errorf("heading = %q %q, want %q %q", h.Heading, h.Tone, c.heading, c.tone)
			}
		})
	}

	// A clock battery running down is a warning, after any reading's.
	for battery, words := range map[string]string{"low": "Clock battery is low", "empty": "Clock battery is empty", "good": "Hardware"} {
		hw := openwrt.HardwareState{Clock: &openwrt.Clock{Part: "pcf2131", Battery: battery}}
		if h := hardwareHeading(identityTranslator, dkInventory(), hardwareAlerts(machine.Facts{}, hw)); h.Heading != words {
			t.Errorf("a %s battery: heading %q, want %q", battery, h.Heading, words)
		}
	}
}

// A board with no profile says so where its photograph would stand, and a
// profile with no photograph says that, so the card keeps its shape; each
// links to the guide to adding a board's profile, its panel art and its
// photograph.
func TestABoardWithoutAPhotoSaysWhy(t *testing.T) {
	guide := `<a href="https://github.com/we-are-mono/verso/blob/main/CONTRIBUTING.md#adding-a-board-profile" class="verso-link" target="_blank" rel="noopener noreferrer">`
	for name, tc := range map[string]struct {
		in    hardwareInput
		title string
		note  string
	}{
		"an unprofiled board":     {unprofiledInput(), "Supermicro H13SAE-MF", "No profile for this board"},
		"a profile with no photo": {func() hardwareInput { in := dkInput(); in.photo = nil; return in }(), "Mono Gateway Development Kit", "No photo of this board"},
	} {
		board := card(t, renderCards(t, tc.in), tc.title)
		note := `<figure class="verso-figure" data-verso-empty><figcaption class="verso-figure__note">` + tc.note + `</figcaption>`
		at := strings.Index(board, note)
		if at < 0 || !strings.Contains(board[at:], guide) || !strings.Contains(board[at:], "How to add one<") {
			t.Errorf("%s does not say %q with the guide under it:\n%s", name, tc.note, board)
		}
	}
}

// The DK's profile ships its photograph, served at an address naming its
// content and kept by the browser for good, as the stylesheet is.
func TestTheBoardPhotoIsKeptForever(t *testing.T) {
	s := newServer(t, fakeBackend{})
	photo := s.boardPhotos["mono_gateway-dk"]
	if photo.href == "" || photo.width != 800 || photo.height != 592 {
		t.Fatalf("the DK's photo = %+v, want it at 800 by 592", photo)
	}
	want, err := fs.ReadFile(profiles.FS, "mono_gateway-dk/board.png")
	if err != nil {
		t.Fatal(err)
	}
	rec := fetchPublic(t, s, photo.href, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("GET %s = %d, %d bytes", photo.href, rec.Code, rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the photo kept for good", got)
	}
	if stale := fetchPublic(t, s, "/assets/boards/000000000000.png", nil); stale.Code != http.StatusNotFound {
		t.Errorf("another build's photo = %d, want 404", stale.Code)
	}
}

// Every profile's photograph is a PNG the page can show, at no more than the
// 800px a third of a page needs on a high-density screen, so none is left
// out at run time or ships heavier than it shows.
func TestEveryBoardPhotoLoads(t *testing.T) {
	files, err := fs.Glob(profiles.FS, "*/board.png")
	if err != nil {
		t.Fatal(err)
	}
	photos := loadBoardPhotos(profiles.FS)
	if len(photos) != len(files) {
		t.Errorf("%d of %d board photos load", len(photos), len(files))
	}
	for dir, photo := range photos {
		if photo.width > 800 {
			t.Errorf("%s/board.png is %dpx wide, want no more than 800", dir, photo.width)
		}
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

// TestTheDKArtKeepsThePanelContract: the DK's rear art is drawn at its own
// size on the 8px grid, its chassis filling its viewBox from the origin, and
// each of its five ports is a group the shell lights: the outline its only
// direct-child path, one link LED and one traffic LED (profiles/README.md).
func TestTheDKArtKeepsThePanelContract(t *testing.T) {
	svg := readPanel("mono_gateway-dk", "back.svg")
	dec := xml.NewDecoder(strings.NewReader(svg))
	type port struct{ paths, links, traffic, roles int }
	ports := map[string]*port{}
	var order []string
	var current *port
	depth, portDepth, chassis := 0, 0, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("back.svg is not well-formed: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			attr := map[string]string{}
			for _, a := range el.Attr {
				attr[a.Name.Local] = a.Value
			}
			switch {
			case el.Name.Local == "svg":
				var w, h int
				if _, err := fmt.Sscanf(attr["viewBox"], "0 0 %d %d", &w, &h); err != nil || w%8 != 0 || h%8 != 0 {
					t.Errorf("viewBox %q should start at 0 0 with a width and height on the 8px grid", attr["viewBox"])
				}
				if attr["width"] != strconv.Itoa(w) || attr["height"] != strconv.Itoa(h) {
					t.Errorf("width/height %s×%s should be the viewBox's own size", attr["width"], attr["height"])
				}
			case el.Name.Local == "rect" && depth == 2 && !chassis:
				// The first drawing is the chassis, filling the box: a 1px
				// stroke half a pixel in.
				chassis = attr["x"] == "0.5" && attr["y"] == "0.5"
				if !chassis {
					t.Errorf("the chassis should fill the viewBox from the origin, got x=%s y=%s", attr["x"], attr["y"])
				}
			case attr["data-verso-port"] != "":
				current = &port{}
				ports[attr["data-verso-port"]] = current
				order = append(order, attr["data-verso-port"])
				portDepth = depth
			case current != nil:
				if el.Name.Local == "path" && depth == portDepth+1 {
					current.paths++
				}
				switch attr["class"] {
				case "led-link":
					current.links++
				case "led-traffic":
					current.traffic++
				case "verso-port-role":
					current.roles++
				}
			}
		case xml.EndElement:
			if current != nil && depth == portDepth {
				current = nil
			}
			depth--
		}
	}
	if want := []string{"eth1", "eth2", "eth0", "eth3", "eth4"}; !slices.Equal(order, want) {
		t.Errorf("ports %v, want %v left to right", order, want)
	}
	for name, p := range ports {
		if p.paths != 1 || p.links != 1 || p.traffic != 1 || p.roles != 1 {
			t.Errorf("%s: %d direct-child paths, %d link and %d traffic LEDs, %d role slots; want one of each", name, p.paths, p.links, p.traffic, p.roles)
		}
	}
}

// The Hardware page wears every page's masthead — the heading line on the
// sand, its title the page's name — and appears as the first System tab, active on its own
// page. The box's own name is the top bar's, so the title does not repeat it.
func TestHardwarePageMastheadAndTab(t *testing.T) {
	srv := newServer(t, fakeBackend{board: openwrt.Board{Model: "Supermicro H13SAE-MF"}})
	body := get(t, srv, "/system/hardware").Body.String()
	for _, want := range []string{
		`<div data-verso-masthead class="verso-page__masthead">`, // every page's heading line on the sand
		`href="/system/hardware"`, `aria-current="page"`,         // the active System tab
		">Supermicro H13SAE-MF</h2>", // the board card
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Hardware page missing %q", want)
		}
	}
	if strings.Contains(body, `verso-page__heading">Supermicro H13SAE-MF`) {
		t.Error("the page's title repeats the box's name the top bar already carries")
	}
	if strings.Contains(body, "All sensors") {
		t.Error("the page still closes on a table of all sensors")
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

// The Temperatures lede says what its bars are only as far as it is true: every
// reading a bar when every one reports a limit, a bar for those that do when
// some do not, and no word of bars when none do.
func TestTheTemperaturesLedeSaysOnlyWhatItsBarsAre(t *testing.T) {
	limited := sensors.TempReading{Name: "cpu", MilliC: 50000, Crit: 95000, HasLimits: true}
	bare := sensors.TempReading{Name: "edge", MilliC: 40000}
	for _, tc := range []struct {
		temps []sensors.TempReading
		want  string
	}{
		{[]sensors.TempReading{limited, limited}, "Each bar runs to the point where the hardware would protect itself."},
		{[]sensors.TempReading{limited, bare}, "A bar runs to the point where the hardware would protect itself; a reading that reports no limit has none."},
		{[]sensors.TempReading{bare, bare}, ""},
	} {
		if got := temperaturesLede(tc.temps); got != tc.want {
			t.Errorf("lede = %q, want %q", got, tc.want)
		}
	}
}

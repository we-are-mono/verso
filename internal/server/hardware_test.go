// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/xml"
	"fmt"
	"io"
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
		facts:   machine.Facts{Cores: 4, MaxKHz: 1600000, MemoryBytes: 8070316 * 1024, Disk: &machine.Disk{Bytes: 61071360 * 512, Kind: "eMMC"}},
		panel:   &widget.Ports{Items: []widget.PortItem{{Label: "eth0", Iface: "eth0"}}},
	}
}

func unprofiledInput() hardwareInput {
	return hardwareInput{
		board: openwrt.Board{Model: "Supermicro H13SAE-MF", BoardName: "supermicro-h13sae-mf", System: "AMD Ryzen 7 7700 8-Core Processor"},
		inv:   unprofiledInventory(),
		facts: machine.Facts{Cores: 16, MemoryBytes: 32 << 30},
		panel: &widget.Ports{Items: []widget.PortItem{{Label: "eth0", Iface: "eth0"}}},
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
	if j := strings.Index(rest, "<section data-verso-section"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// The page is the board card, then one card per kind of reading, in order.
func TestHardwareCardsStandInOrder(t *testing.T) {
	got := cardTitles(renderCards(t, dkInput()))
	want := []string{"Mono Gateway Development Kit", "Temperatures", "Power", "Cooling", "Fiber modules"}
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
// "Ports", so the panel never stands under an empty heading.
func TestTheBoardCardsTitle(t *testing.T) {
	if got := cardTitles(renderCards(t, dkInput()))[0]; got != "Mono Gateway Development Kit" {
		t.Errorf("profiled title %q", got)
	}
	if got := cardTitles(renderCards(t, unprofiledInput()))[0]; got != "Supermicro H13SAE-MF" {
		t.Errorf("unprofiled title %q", got)
	}
	in := unprofiledInput()
	in.board = openwrt.Board{}
	if got := cardTitles(renderCards(t, in))[0]; got != "Ports" {
		t.Errorf("a board that says nothing of itself is titled %q", got)
	}
}

// The board card says what the box is made of, a fact it cannot answer left
// out.
func TestTheBoardCardsFacts(t *testing.T) {
	board := card(t, renderCards(t, dkInput()), "Mono Gateway Development Kit")
	for _, want := range []string{
		"verso-ports",      // the panel
		`data-verso-facts`, // two columns where the card is wide
		">NXP LS1046A · 4 cores · 1.6\U0000202fGHz<", // the profile's chip, the kernel's cores and clock
		">7.7\U0000202fGiB<",
		">29\U0000202fGiB eMMC<", // whole GiB from ten up, as every size in the shell
		">mono,gateway-dk<",
	} {
		if !strings.Contains(board, want) {
			t.Errorf("board card missing %q:\n%s", want, board)
		}
	}

	board = card(t, renderCards(t, unprofiledInput()), "Supermicro H13SAE-MF")
	if !strings.Contains(board, ">AMD Ryzen 7 7700 8-Core Processor · 16 cores<") {
		t.Errorf("an unprofiled processor reads as the kernel names it, with its cores:\n%s", board)
	}
	if strings.Contains(board, "Storage") || strings.Contains(board, "GHz") {
		t.Errorf("a fact the box cannot answer should be left out:\n%s", board)
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
		">Show chart for<", `class="verso-joined-word">over</span>`,
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

// Each row's bar runs to its protect limit with ticks at both limits, and its
// Limits say them.
func TestATemperatureShowsItsHeadroom(t *testing.T) {
	temps := card(t, renderCards(t, dkInput()), "Temperatures")
	for _, want := range []string{
		`bg-marigold-deep" style="left: 89%"`,             // warns at 85 of 95
		`bg-crimson" style="left: calc(100% - 0.125rem)"`, // protects at the bar's end
		`clip-path: inset(0 50% 0 0 round 9999px)`,        // 48 of 95
		">48.0\U0000202f°C<",
		">warns 85\U0000202f°C · protects 95\U0000202f°C<",
	} {
		if !strings.Contains(temps, want) {
			t.Errorf("Temperatures missing %q", want)
		}
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
	if !strings.Contains(cooling, ">Stalled<") || !strings.Contains(cooling, "bg-marigold") {
		t.Errorf("a stalled fan should read Stalled in the warning tone:\n%s", cooling)
	}
}

// Each SFP module is a headed fact list, as the Overview heads an address
// family, never a card inside the card.
func TestEachModuleIsAHeadedList(t *testing.T) {
	fibre := card(t, renderCards(t, dkInput()), "Fiber modules")
	for _, want := range []string{"data-verso-properties-head", ">SFP&#43; module<", ">xfi0<", ">xfi1<", ">Receiving light<", ">No signal<"} {
		if !strings.Contains(fibre, want) {
			t.Errorf("Fiber modules missing %q", want)
		}
	}
	if strings.Contains(fibre, "data-verso-artifact") {
		t.Errorf("a module stands as a card inside the card:\n%s", fibre)
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
		heading, tone string
	}{
		{"all well", func(*sensors.Inventory) {}, "Hardware", "neutral"},
		{"a hot CPU", func(inv *sensors.Inventory) { inv.Temps[0].MilliC, inv.Temps[0].Level = 87000, "warn" }, "cluster is hot", "warning"},
		{"a critical CPU beside a stopped fan", func(inv *sensors.Inventory) {
			inv.Temps[0].MilliC, inv.Temps[0].Level = 96000, "critical"
			inv.Fans[0].State, inv.Fans[0].RPM = "fault", 0
		}, "cluster is hot", "danger"},
		{"a stopped fan", func(inv *sensors.Inventory) { inv.Fans[0].State, inv.Fans[0].RPM = "fault", 0 }, "System Fan 1 stopped", "warning"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv := dkInventory()
			c.edit(&inv)
			if h := hardwareHeading(identityTranslator, inv); h.Heading != c.heading || h.Tone != c.tone {
				t.Errorf("heading = %q %q, want %q %q", h.Heading, h.Tone, c.heading, c.tone)
			}
		})
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
		`<div data-verso-masthead class="py-6">`,         // every page's heading line on the sand
		`href="/system/hardware"`, `aria-current="page"`, // the active System tab
		">Supermicro H13SAE-MF</h2>", // the board card
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Hardware page missing %q", want)
		}
	}
	if strings.Contains(body, `verso-page-heading">Supermicro H13SAE-MF`) {
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

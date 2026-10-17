// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/machine"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/units"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// handleSystemHardware renders the Hardware page: the board card (its name,
// its panel, what it is made of), then one card per kind of reading the board
// has: Temperatures, Power and Cooling, then a card per fiber module, each
// reading once. It
// reads local sysfs and procfs only (no network, no blocking), so it renders
// fresh on each load, with no cache. The panel stays live from the overview
// stream's interfaces frame; the readings are as loaded.
func (s *Server) handleSystemHardware(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	board, boardErr := s.backend.Board(r.Context(), sid)
	if boardErr != nil {
		log.Printf("verso: hardware: board unavailable: %v", boardErr)
	}
	profile, _ := sensors.LoadProfile(profiles.FS, board.BoardName)
	in := hardwareInput{
		board:   board,
		profile: profile,
		inv:     s.sensorInventory(profile),
		facts:   (&machine.Reader{}).Read(),
		hw:      s.hardwareState(r.Context(), sid),
		panel:   s.hardwarePanel(r, profile),
	}

	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	// The page opens on the first reading's day, drawn here, so the graph
	// needs no second request.
	if choices := temperatureChoices(tr, in.inv); len(choices) > 0 {
		in.tempGraph = s.temperatureGraph(r.Context(), tr, choices[0], historyWindows[0])
	}
	root := &widget.Stack{Children: hardwareCards(tr, in)}

	// Wide, as every page of listings: the Temperatures table's bar takes what
	// its name, source and limits leave.
	s.renderWidgetPage(w, r, http.StatusOK, hardwareHeading(tr, in.inv, hardwareAlerts(in.facts, in.hw)),
		"wide", s.sectionPages("System", r.URL.Path), root)
}

// hardwareInput is everything the page is drawn from.
type hardwareInput struct {
	board   openwrt.Board
	profile *sensors.Profile
	inv     sensors.Inventory
	facts   machine.Facts
	hw      openwrt.HardwareState // what only root may read: the clock's battery, U-Boot's slot
	panel   widget.Widget         // nil for a box with no ports
	// tempGraph is the Temperatures graph as the page opens on it: the first
	// reading (the CPU) over the last 24 hours. Without it the card is its
	// table alone.
	tempGraph widget.Widget
}

// hardwareHeading is the page's heading: "Hardware" while all is well, else
// what needs attention in its tone, in the words the Overview's Hardware card
// speaks, from the same grading.
func hardwareHeading(tr func(string) string, inv sensors.Inventory, alerts []widget.OverviewAlert) pageHeader {
	if tone, words := hardwareView(inv, alerts).Verdict(tr); tone != "" {
		return pageHeader{Heading: words, Tone: tone}
	}
	return pageHeader{Heading: "Hardware", Tone: "neutral"}
}

// hardwareCards composes the page: the board card, then each kind of reading
// the board has, in a card of its own. A kind with no reading has no card.
func hardwareCards(tr func(string) string, in hardwareInput) []widget.Widget {
	nums := units.For(tr)
	var out []widget.Widget
	for _, card := range []widget.Widget{
		boardCard(tr, nums, in),
		temperaturesCard(tr, in),
		powerCard(in.inv.PageRails()),
		coolingCard(in.inv.PageFans()),
		fibreCards(in.inv.Fibers),
	} {
		if card != nil {
			out = append(out, card)
		}
	}
	return out
}

// hardwarePanel renders the router's panel (port_panel.go). A box with no
// ports shows nothing.
func (s *Server) hardwarePanel(r *http.Request, profile *sensors.Profile) widget.Widget {
	snapshot, _ := s.telemetrySnapshot(r.Context())
	wan, _ := s.wanStatus(r)
	if panel := s.devicePanel(snapshot, wan, profile); panel != nil {
		return panel
	}
	return nil
}

// readPanel loads a profile folder's embedded panel artwork, empty when the folder
// ships none. The art is first-party content compiled into the binary, so the
// Hardware page renders it as trusted core content (the runtime-contribution
// sanitizer the profile README specifies is only needed once profiles arrive at
// runtime). dir is the profile's own folder (LoadProfile records it), so the art
// travels with the profile, not with the raw board_name.
func readPanel(dir, file string) string {
	if dir == "" {
		return ""
	}
	data, err := fs.ReadFile(profiles.FS, dir+"/"+file)
	if err != nil {
		return ""
	}
	return string(data)
}

// --- the board card --------------------------------------------------------

// boardCard names the router (the profile's name, else its model, else
// "Ports", so the panel never stands under an empty heading) and holds what
// it is made of at the left, its panel beside it on the right.
func boardCard(tr func(string) string, nums units.Numbers, in hardwareInput) widget.Widget {
	title := "Ports"
	switch {
	case in.profile != nil && in.profile.Name != "":
		title = in.profile.Name
	case in.board.Model != "":
		title = in.board.Model
	}
	var children []widget.Widget
	if facts := machineFacts(tr, nums, in); facts != nil {
		children = append(children, facts)
	}
	if in.panel != nil {
		children = append(children, in.panel)
	}
	switch len(children) {
	case 0:
		return nil
	case 2:
		children = []widget.Widget{&widget.Grid{Style: "figure", Children: children}}
	}
	return &widget.Section{Title: title, Children: children}
}

// machineFacts is what the box is made of, one list read down: Processor,
// Memory, Storage, Boot slot, NOR flash, Clock and Board ID (the board_name
// the device tree or the firmware's DMI gives, which firmware images are
// matched by; an identifier, not a name). A fact the box cannot answer is
// left out.
func machineFacts(tr func(string) string, nums units.Numbers, in hardwareInput) widget.Widget {
	var items []widget.Property
	if v := processorFact(tr, nums, in); v != "" {
		items = append(items, widget.Property{Label: "Processor", Value: v, Verbatim: true})
	}
	if in.facts.MemoryBytes > 0 {
		items = append(items, widget.Property{Label: "Memory", Value: nums.Bytes(in.facts.MemoryBytes), Verbatim: true})
	}
	if d := in.facts.Disk; d != nil {
		items = append(items, storageFact(tr, nums, d))
	}
	if slot := in.hw.BootSlot(in.facts.Root); slot != nil {
		items = append(items, widget.Property{Label: "Boot slot", Value: bootSlotFact(tr, slot), Verbatim: true})
	}
	if f := in.facts.Flash; f != nil {
		items = append(items, widget.Property{Label: "NOR flash", Value: flashFact(nums, f), Verbatim: true})
	}
	if c := in.hw.Clock; c != nil {
		items = append(items, clockFact(tr, c))
	}
	if in.board.BoardName != "" {
		items = append(items, widget.Property{Label: "Board ID", Value: in.board.BoardName, Mono: true})
	}
	if len(items) == 0 {
		return nil
	}
	return &widget.Properties{Align: "left", Items: items}
}

// processorFact is the chip (the profile's, else the kernel's name for it),
// its cores, and its top clock where cpufreq reports one.
func processorFact(tr func(string) string, nums units.Numbers, in hardwareInput) string {
	var parts []string
	switch {
	case in.profile != nil && in.profile.Processor != "":
		parts = append(parts, in.profile.Processor)
	case in.board.System != "":
		parts = append(parts, in.board.System)
	}
	switch n := in.facts.Cores; {
	case n == 1:
		parts = append(parts, tr("1 core"))
	case n > 1:
		parts = append(parts, fmt.Sprintf(tr("%d cores"), n))
	}
	if khz := in.facts.MaxKHz; khz > 0 {
		parts = append(parts, nums.Figure(float64(khz)/1e6)+units.Space+"GHz")
	}
	return strings.Join(parts, " · ")
}

// storageFact is the boot disk's size and kind, then the wear an eMMC
// estimates for itself, in tenths of its rated life; its row is marked in the
// tone the heading speaks it in.
func storageFact(tr func(string) string, nums units.Numbers, d *machine.Disk) widget.Property {
	p := widget.Property{Label: "Storage", Value: nums.Bytes(d.Bytes) + " " + tr(d.Kind), Verbatim: true}
	if w := d.Wear; w != nil && w.Band > 0 {
		worn := tr("past its rated wear")
		if w.Band <= 10 {
			worn = fmt.Sprintf(tr("%s worn"), fmt.Sprintf("%d–%d", (w.Band-1)*10, w.Band*10)+units.Space+"%")
		}
		p.Value += " · " + worn
		if level := w.Level(); level != "" {
			p.Dot = tempVariant(level)
		}
	}
	return p
}

// bootSlotFact is the slot the kernel booted, then the slot U-Boot boots next
// when a switch is staged, and a new image's boots so far on trial.
func bootSlotFact(tr func(string) string, s *openwrt.BootSlot) string {
	parts := []string{strings.ToUpper(s.Running)}
	if s.Next != s.Running {
		parts = append(parts, fmt.Sprintf(tr("%s on next boot"), strings.ToUpper(s.Next)))
	}
	if s.Trial {
		if s.Limit > 0 {
			parts = append(parts, fmt.Sprintf(tr("trial boot %d of %d"), s.Count, s.Limit))
		} else {
			parts = append(parts, tr("on trial"))
		}
	}
	return strings.Join(parts, " · ")
}

// clockFact is the clock the system time was set from at boot, as its device
// tree names its part, then its backup battery where it says; a battery
// running down marks the row in the warning tone the heading speaks it in.
func clockFact(tr func(string) string, c *openwrt.Clock) widget.Property {
	p := widget.Property{Label: "Clock", Value: c.Part, Verbatim: true}
	switch c.Battery {
	case "good":
		p.Value += " · " + tr("battery good")
	case "low":
		p.Value, p.Dot = p.Value+" · "+tr("battery low"), "warning"
	case "empty":
		p.Value, p.Dot = p.Value+" · "+tr("battery empty"), "warning"
	}
	return p
}

// flashFact is the NOR chip as the kernel names its part, then its size; a
// chip it names none is its size alone.
func flashFact(nums units.Numbers, f *machine.Flash) string {
	if f.Part == "" {
		return nums.Bytes(f.Bytes)
	}
	return f.Part + " · " + nums.Bytes(f.Bytes)
}

// --- the kinds of reading --------------------------------------------------

// temperaturesCard is the board's temperatures: one reading's history,
// chosen in the field over it, then one table of every reading, each a
// headroom bar to the point where the hardware would protect itself.
func temperaturesCard(tr func(string) string, in hardwareInput) widget.Widget {
	temps := in.inv.PageTemps()
	if len(temps) == 0 {
		return nil
	}
	var children []widget.Widget
	if choices := temperatureChoices(tr, in.inv); in.tempGraph != nil && len(choices) > 0 {
		children = append(children, temperatureField(choices, choices[0].Series, historyWindows[0].Value), in.tempGraph)
	}
	rows := make([]widget.TableRow, 0, len(temps))
	for i := range temps {
		t := &temps[i]
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: t.Name, Verbatim: true, Muted: t.Fault},
			{Text: tempSource(t), Verbatim: true, Muted: t.Fault},
			tempMeter(t),
			{Text: tempLimits(tr, t), Verbatim: true, Muted: t.Fault},
		}})
	}
	return &widget.Section{
		Title: "Temperatures", Hairline: true,
		Sub: temperaturesLede(temps),
		Children: append(children, &widget.Table{
			Columns: []widget.TableColumn{
				{Label: "Reading", Kind: "name", Width: widget.MeasureName},
				{Label: "Source", Kind: "mono", Width: widget.MeasureAddress},
				{Label: "Temperature", Kind: "meter"},
				{Label: "Limits", Kind: "comment"},
			},
			Rows: rows,
		}),
	}
}

// temperaturesLede says what the card's bars are, only as far as it is true:
// every reading a bar when every one reports a limit, a bar for those that do
// when some do not, and nothing of bars when none do.
func temperaturesLede(temps []sensors.TempReading) string {
	barred := 0
	for i := range temps {
		if hasBar(&temps[i]) {
			barred++
		}
	}
	switch {
	case barred == 0:
		return ""
	case barred == len(temps):
		return "Each bar runs to the point where the hardware would protect itself."
	}
	return "A bar runs to the point where the hardware would protect itself; a reading that reports no limit has none."
}

// hasBar reports whether a reading draws a bar: it reads, and it reports a
// limit to run the bar to.
func hasBar(t *sensors.TempReading) bool {
	return !t.Fault && (t.Crit > 0 || t.Warn > 0)
}

// tempSource is where a temperature is read: a zone's type, or a chip and the
// channel it names, the channel left off where the reading is already named
// for it.
func tempSource(t *sensors.TempReading) string {
	if t.Kind == "zone" || t.Source == "" {
		return t.Kernel
	}
	if t.Kernel == t.Name {
		return t.Source
	}
	return t.Source + " " + t.Kernel
}

// tempMeter fills a reading's bar to its own protect limit (its warn limit
// when that is all it has), ticked at each; a reading with no limit has no
// bar, never one against a guessed ceiling, and a faulted channel no reading.
func tempMeter(t *sensors.TempReading) widget.TableCell {
	if t.Fault {
		return widget.TableCell{Text: "—", Muted: true}
	}
	cell := widget.TableCell{Text: celsiusOne(t.MilliC) + units.Space + "°C", Verbatim: true}
	ceiling := t.Crit
	if ceiling == 0 {
		ceiling = t.Warn
	}
	if ceiling == 0 {
		cell.NoTrack = true
		return cell
	}
	cell.Fill = percent(t.MilliC, ceiling)
	if t.Warn > 0 {
		cell.WarnMark = percent(t.Warn, ceiling)
	}
	if t.Crit > 0 {
		cell.CritMark = 100
	}
	switch t.Level {
	case "warn":
		cell.Variant = "warning"
	case "critical":
		cell.Variant = "danger"
	}
	return cell
}

// tempLimits says a reading's limits, or why it has none.
func tempLimits(tr func(string) string, t *sensors.TempReading) string {
	switch {
	case t.Fault:
		return tr("sensor fault")
	case t.Warn > 0 && t.Crit > 0:
		return fmt.Sprintf(tr("warns %s · protects %s"), celsiusRound(t.Warn), celsiusRound(t.Crit))
	case t.Crit > 0:
		return fmt.Sprintf(tr("protects %s"), celsiusRound(t.Crit))
	case t.Warn > 0:
		return fmt.Sprintf(tr("warns %s"), celsiusRound(t.Warn))
	default:
		// A driver's sentinel standing in for a limit is no limit, and reads
		// as one.
		return tr("no limits reported")
	}
}

// powerCard is one table of the board's rails, the input first and tagged as
// it: its row states the draw, so the heading carries no total.
func powerCard(rails []sensors.PowerReading) widget.Widget {
	if len(rails) == 0 {
		return nil
	}
	rows := make([]widget.TableRow, 0, len(rails))
	for _, p := range rails {
		name := widget.TableCell{Text: p.Name, Verbatim: true}
		if p.Main {
			name.Tag = "input"
		}
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			name,
			{Text: p.Source, Verbatim: true},
			{Text: voltage(p), Verbatim: true},
			{Text: current(p), Verbatim: true},
			{Text: powerText(p.MicroW), Verbatim: true},
		}})
	}
	return &widget.Section{
		Title: "Power", Hairline: true,
		Children: []widget.Widget{&widget.Table{
			Columns: []widget.TableColumn{
				{Label: "Rail", Kind: "name", Width: widget.MeasureName},
				{Label: "Source", Kind: "mono"},
				{Label: "Voltage", Kind: "num"},
				{Label: "Current", Kind: "num"},
				{Label: "Power", Kind: "num"},
			},
			Rows: rows,
		}},
	}
}

// coolingCard is one table of the board's fans, one fan as several, each
// fan's state last.
func coolingCard(fans []sensors.FanReading) widget.Widget {
	if len(fans) == 0 {
		return nil
	}
	rows := make([]widget.TableRow, 0, len(fans))
	for i := range fans {
		f := &fans[i]
		speed, duty := "—", ""
		if f.State == "running" {
			speed = rpmGroup(f.RPM) + units.Space + "rpm"
			if f.HasDuty {
				duty = fmt.Sprintf("%d"+units.Space+"%%", f.Duty)
			}
		}
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: f.Name, Verbatim: true, Muted: f.State == "absent"},
			{Text: f.Source + " " + f.Kernel, Verbatim: true, Muted: f.State == "absent"},
			{Text: speed, Verbatim: true, Muted: f.State != "running"},
			{Text: duty, Verbatim: true},
			fanStatus(f.State),
		}})
	}
	return &widget.Section{
		Title: "Cooling", Hairline: true,
		Children: []widget.Widget{&widget.Table{
			Columns: []widget.TableColumn{
				{Label: "Fan", Kind: "name", Width: widget.MeasureName},
				{Label: "Source", Kind: "mono"},
				{Label: "Speed", Kind: "num"},
				{Label: "Duty", Kind: "num"},
				{Label: "Status", Kind: "status"},
			},
			Rows: rows,
		}},
	}
}

// fanStatus is a fan's state: spinning, stalled with its alarm raised, or an
// unpopulated header, which is quiet.
func fanStatus(state string) widget.TableCell {
	switch state {
	case "running":
		return widget.TableCell{Text: "Running", Variant: "success"}
	case "fault":
		return widget.TableCell{Text: "Stalled", Variant: "warning"}
	default:
		return widget.TableCell{Text: "Not connected"}
	}
}

// fibreCards is a card per fiber module, side by side, titled "SFP+ module"
// with its cage's chip in the header, the optical pair leading its facts
// (dBm, with raw microwatts as the quiet twin) under a plain link verdict.
func fibreCards(fibers []sensors.FiberModule) widget.Widget {
	if len(fibers) == 0 {
		return nil
	}
	cards := make([]widget.Widget, 0, len(fibers))
	for i := range fibers {
		cards = append(cards, &widget.Section{
			Title: "SFP+ module", Chip: fibers[i].Cage, Hairline: true,
			Children: []widget.Widget{fibreFacts(&fibers[i])},
		})
	}
	return &widget.Grid{Style: "cards", Children: cards}
}

func fibreFacts(f *sensors.FiberModule) widget.Widget {
	link := widget.Property{Label: "Link", Value: "Receiving light", Dot: "success"}
	if !f.RxHasLight {
		link = widget.Property{Label: "Link", Value: "No signal", Dot: "warning"}
	}
	// The readings are figures with their units, so they are data in the sans,
	// as every other figure is; mono is for strings someone could type back in.
	items := []widget.Property{link}
	if f.HasTx {
		items = append(items, widget.Property{Label: "Transmit", Value: opticalPair(f.TxMicroW, true), Verbatim: true})
	}
	if f.HasRx {
		items = append(items, widget.Property{Label: "Receive", Value: opticalPair(f.RxMicroW, f.RxHasLight), Verbatim: true})
	}
	items = append(items, widget.Property{Label: "Temperature", Value: celsiusOne(f.TempMilliC) + units.Space + "°C", Verbatim: true})
	if f.HasVcc {
		items = append(items, widget.Property{Label: "Supply", Value: fmt.Sprintf("%.2f"+units.Space+"V", float64(f.VccMilliV)/1000), Verbatim: true})
	}
	if f.HasBias {
		items = append(items, widget.Property{Label: "Laser bias", Value: fmt.Sprintf("%d"+units.Space+"mA", f.BiasMilliA), Verbatim: true})
	}
	return &widget.Properties{Align: "left", Items: items}
}

// --- unit formatting -------------------------------------------------------

func voltage(p sensors.PowerReading) string {
	if !p.HasV {
		return ""
	}
	return fmt.Sprintf("%.2f"+units.Space+"V", float64(p.MilliV)/1000)
}

func current(p sensors.PowerReading) string {
	if !p.HasA {
		return ""
	}
	return fmt.Sprintf("%.2f"+units.Space+"A", float64(p.MilliA)/1000)
}

// celsiusRound renders a milli-°C as a whole degree ("85 °C"); celsiusOne keeps
// one decimal ("47.0").
func celsiusRound(milliC int) string { return fmt.Sprintf("%d"+units.Space+"°C", (milliC+500)/1000) }

// celsiusWhole is the rounded degrees with no unit, for a figure that carries
// its "°C" beside it.
func celsiusWhole(milliC int) string { return fmt.Sprintf("%d", (milliC+500)/1000) }

func celsiusOne(milliC int) string { return fmt.Sprintf("%.1f", float64(milliC)/1000) }

// powerText renders a channel's draw in the unit that keeps its figure legible —
// milliwatts for the sub-watt channels a GPU reports, watts for a rail.
func powerText(microW int64) string {
	if microW != 0 && microW > -1_000_000 && microW < 1_000_000 {
		return fmt.Sprintf("%d"+units.Space+"mW", microW/1000)
	}
	return fmt.Sprintf("%.1f"+units.Space+"W", float64(microW)/1e6)
}

// optical renders a microwatt reading the way a module reports it — plain
// microwatts under a milliwatt, milliwatts above.
func optical(microW int64) string {
	if microW >= 1000 {
		return fmt.Sprintf("%.2f"+units.Space+"mW", float64(microW)/1000)
	}
	return fmt.Sprintf("%d"+units.Space+"µW", microW)
}

// opticalPair renders an optical power as dBm (the reading engineers read) with the
// raw power as a quiet twin; a receive channel below its floor reads "no light".
func opticalPair(microW int64, hasLight bool) string {
	if !hasLight {
		return fmt.Sprintf("%s · no light", dbm(microW))
	}
	return fmt.Sprintf("%s · %s", dbm(microW), optical(microW))
}

// dbm converts microwatts to dBm (0 dBm = 1 mW = 1000 µW); a dark channel is
// reported as a floor, not −∞.
func dbm(microW int64) string {
	if microW <= 0 {
		return "< −40" + units.Space + "dBm"
	}
	return fmt.Sprintf("%.1f"+units.Space+"dBm", 10*math.Log10(float64(microW)/1000))
}

func rpmGroup(rpm int) string {
	s := fmt.Sprintf("%d", rpm)
	if len(s) <= 3 {
		return s
	}
	head := len(s) % 3
	var b strings.Builder
	if head > 0 {
		b.WriteString(s[:head])
	}
	for i := head; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// percent scales a reading to its ceiling as a 0–100 percent for a track fill or
// tick position.
func percent(value, ceiling int) int {
	if ceiling <= 0 {
		return 0
	}
	p := value * 100 / ceiling
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func tempVariant(level string) string {
	switch level {
	case "warn":
		return "warning"
	case "critical":
		return "danger"
	case "nominal":
		return "success"
	default:
		return "neutral"
	}
}

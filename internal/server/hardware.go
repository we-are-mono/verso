// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
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
// has: Temperatures, Power, Cooling and Fiber modules, each reading once. It
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
		panel:   s.hardwarePanel(r, profile),
	}

	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	// The page opens on the first reading's day, drawn here, so the graph
	// needs no second request.
	if choices := temperatureChoices(tr, in.inv); len(choices) > 0 {
		in.tempGraph = s.temperatureGraph(r.Context(), tr, choices[0], historyWindows[0])
	}
	root := &widget.Stack{Children: hardwareCards(tr, in)}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, root, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Wide, as every page of listings: the Temperatures table's bar takes what
	// its name, source and limits leave.
	s.renderPage(w, r, http.StatusOK, hardwareHeading(tr, in.inv),
		"wide", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

// hardwareInput is everything the page is drawn from.
type hardwareInput struct {
	board   openwrt.Board
	profile *sensors.Profile
	inv     sensors.Inventory
	facts   machine.Facts
	panel   widget.Widget // nil for a box with no ports
	// tempGraph is the Temperatures graph as the page opens on it: the first
	// reading (the CPU) over the last 24 hours. Without it the card is its
	// table alone.
	tempGraph widget.Widget
}

// hardwareHeading is the page's heading: "Hardware" while all is well, else
// what needs attention in its tone, in the words the Overview's Hardware card
// speaks, from the same grading.
func hardwareHeading(tr func(string) string, inv sensors.Inventory) pageHeader {
	if tone, words := hardwareView(inv).Verdict(tr); tone != "" {
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
		fibreCard(in.inv.Fibers),
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
// "Ports", so the panel never stands under an empty heading) and holds its
// panel, then what it is made of in two columns.
func boardCard(tr func(string) string, nums units.Numbers, in hardwareInput) widget.Widget {
	title := "Ports"
	switch {
	case in.profile != nil && in.profile.Name != "":
		title = in.profile.Name
	case in.board.Model != "":
		title = in.board.Model
	}
	var children []widget.Widget
	if in.panel != nil {
		children = append(children, in.panel)
	}
	if facts := machineFacts(tr, nums, in); facts != nil {
		children = append(children, facts)
	}
	if len(children) == 0 {
		return nil
	}
	return &widget.Section{Title: title, Children: children}
}

// machineFacts is what the box is made of: Processor and Memory, then Storage
// and Board name. A fact the box cannot answer is left out.
func machineFacts(tr func(string) string, nums units.Numbers, in hardwareInput) widget.Widget {
	var left, right []widget.Property
	if v := processorFact(tr, nums, in); v != "" {
		left = append(left, widget.Property{Label: "Processor", Value: v, Verbatim: true})
	}
	if in.facts.MemoryBytes > 0 {
		left = append(left, widget.Property{Label: "Memory", Value: nums.Bytes(in.facts.MemoryBytes), Verbatim: true})
	}
	if d := in.facts.Disk; d != nil {
		right = append(right, widget.Property{Label: "Storage", Value: nums.Bytes(d.Bytes) + " " + tr(d.Kind), Verbatim: true})
	}
	if in.board.BoardName != "" {
		right = append(right, widget.Property{Label: "Board name", Value: in.board.BoardName, Mono: true})
	}
	if len(left)+len(right) == 0 {
		return nil
	}
	return &widget.Grid{Style: "facts", Children: []widget.Widget{
		&widget.Properties{Align: "left", Items: left},
		&widget.Properties{Align: "left", Items: right},
	}}
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
		Sub: "Each bar runs to the point where the hardware would protect itself.",
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

// fibreCard heads each SFP module's facts as the Overview heads an address
// family: "SFP+ module" with its cage as a chip, the optical pair leading
// (dBm, with raw microwatts as the quiet twin) under a plain link verdict.
func fibreCard(fibers []sensors.FiberModule) widget.Widget {
	if len(fibers) == 0 {
		return nil
	}
	lists := make([]widget.Widget, 0, len(fibers))
	for i := range fibers {
		lists = append(lists, fibreFacts(&fibers[i]))
	}
	return &widget.Section{
		Title: "Fiber modules", Hairline: true,
		Children: []widget.Widget{&widget.Grid{Columns: 2, Children: lists}},
	}
}

func fibreFacts(f *sensors.FiberModule) widget.Widget {
	link := widget.Property{Label: "Link", Value: "Receiving light", Dot: "success"}
	if !f.RxHasLight {
		link = widget.Property{Label: "Link", Value: "No signal", Dot: "warning"}
	}
	items := []widget.Property{link}
	if f.HasTx {
		items = append(items, widget.Property{Label: "Transmit", Value: opticalPair(f.TxMicroW, true), Mono: true})
	}
	if f.HasRx {
		items = append(items, widget.Property{Label: "Receive", Value: opticalPair(f.RxMicroW, f.RxHasLight), Mono: true})
	}
	items = append(items, widget.Property{Label: "Temperature", Value: celsiusOne(f.TempMilliC) + units.Space + "°C", Mono: true})
	if f.HasVcc {
		items = append(items, widget.Property{Label: "Supply", Value: fmt.Sprintf("%.2f"+units.Space+"V", float64(f.VccMilliV)/1000), Mono: true})
	}
	if f.HasBias {
		items = append(items, widget.Property{Label: "Laser bias", Value: fmt.Sprintf("%d"+units.Space+"mA", f.BiasMilliA), Mono: true})
	}
	return &widget.Properties{
		Align: "left",
		Head:  &widget.PropertiesHead{Label: "SFP+ module", Chip: f.Cage},
		Items: items,
	}
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

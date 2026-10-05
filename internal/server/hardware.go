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

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// handleSystemHardware renders the Hardware page: the device's own model, panel,
// and every temperature, power rail, fan, and fibre module the kernel exposes,
// resolved through the board profile. It reads local sysfs only (no network, no
// blocking), so it renders fresh on each load — no cache, no stream needed for
// correctness. Live glide of the readings and the panel LED blink are a follow-up.
func (s *Server) handleSystemHardware(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	board, boardErr := s.backend.Board(r.Context(), sid)
	if boardErr != nil {
		log.Printf("verso: hardware: board unavailable: %v", boardErr)
	}
	profile, _ := sensors.LoadProfile(profiles.FS, board.BoardName)
	inv := sensors.ResolveAll(profile, &sensors.Reader{})

	root := &widget.Stack{Children: s.hardwareBody(r, board, profile, inv)}

	var body strings.Builder
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, root, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Hardware", Tone: "neutral"},
		"narrow", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

// hardwareBody composes the page's widget tree: the panel, the vitals grid, the
// curated detail sections the hardware can fill, and the full readings tables
// under the page-wide filter.
func (s *Server) hardwareBody(r *http.Request, board openwrt.Board, profile *sensors.Profile, inv sensors.Inventory) []widget.Widget {
	var out []widget.Widget

	if panel := s.hardwarePanel(r, board, profile); panel != nil {
		out = append(out, panel)
	}
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if grid := hardwareVitals(tr, inv); grid != nil {
		out = append(out, grid)
	}
	if temps := hardwareTemps(tr, profile, inv); temps != nil {
		out = append(out, temps)
	}
	if power := hardwarePower(tr, inv); power != nil {
		out = append(out, power)
	}
	if fans := hardwareFans(tr, inv); fans != nil {
		out = append(out, fans)
	}
	if fibre := hardwareFibre(inv); fibre != nil {
		out = append(out, fibre)
	}
	out = append(out, hardwareReadings(inv)...)
	return out
}

// hardwarePanel renders the rear panel: the profile's own back.svg (lit from live
// port state) when it ships one, else the generated connector strip drawn from the
// box's physical interfaces. A box with no ports at all shows nothing.
func (s *Server) hardwarePanel(r *http.Request, board openwrt.Board, profile *sensors.Profile) widget.Widget {
	snapshot, _ := s.telemetrySnapshot(r.Context())
	wan, _ := s.backend.WANStatus(r.Context(), s.sessionSID(r))
	items := panelPorts(profile, snapshot, wan)
	if len(items) == 0 {
		return nil
	}
	p := &widget.Ports{Items: items, Legend: true}
	if profile != nil {
		p.Back = readPanel(profile.Dir, "back.svg")
		p.Front = readPanel(profile.Dir, "front.svg")
	}
	return p
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

// panelPorts builds the panel's port list — ordered by the profile's left-to-right
// ports when one exists, else the box's physical interfaces — carrying each port's
// live link, traffic, and WAN role so the panel lights from real state.
func panelPorts(profile *sensors.Profile, snapshot telemetry.Snapshot, wan openwrt.WANState) []widget.PortItem {
	wanDevice := map[string]bool{}
	for _, d := range wan.Devices {
		wanDevice[d.Device] = true
	}
	order := []string{}
	if profile != nil && len(profile.Ports) > 0 {
		order = profile.Ports
	} else {
		for _, iface := range snapshot.Interfaces {
			if iface.Physical && iface.Name != "lo" && iface.Kind != "wireless" {
				order = append(order, iface.Name)
			}
		}
	}
	items := make([]widget.PortItem, 0, len(order))
	for _, name := range order {
		linked, active := false, false
		if iface, ok := snapshot.Interface(name); ok {
			linked = iface.Operstate == "up"
			if n := len(iface.History); n > 0 {
				point := iface.History[n-1]
				active = point.RxBPS > 0 || point.TxBPS > 0
			}
		}
		role := ""
		if wanDevice[name] {
			role = "wan"
		}
		items = append(items, widget.PortItem{
			Kind: "rj45", Label: name, Verbatim: true, Iface: name, Linked: linked, Active: active, Role: role,
		})
	}
	return items
}

// hardwareVitals is the headline instrument grid — the CPU temperature, the main
// power draw, and the main fan, each pulled out of the detail below. A box that
// reports less shows fewer tiles; when it reports neither power nor a fan, the
// warmest sensor stands beside the CPU so the grid is never a lonely single tile.
func hardwareVitals(tr func(string) string, inv sensors.Inventory) widget.Widget {
	var tiles []widget.Widget
	if cpu := inv.CPUTemp(); cpu != nil {
		tiles = append(tiles, cpuTile(tr, cpu))
	}
	if p := inv.MainPower(); p != nil {
		tiles = append(tiles, powerTile(tr, p))
	}
	if f := inv.MainFan(); f != nil {
		tiles = append(tiles, fanTile(tr, f))
	}
	if len(tiles) == 1 {
		if warm := warmestTile(tr, inv); warm != nil {
			tiles = append(tiles, warm)
		}
	}
	if len(tiles) == 0 {
		return nil
	}
	return &widget.Grid{Style: "strip", Columns: len(tiles), Children: tiles}
}

func cpuTile(tr func(string) string, t *sensors.TempReading) widget.Widget {
	// Composed prose translates its format at the point of composition — the
	// schema walk can only match whole catalog keys ("warns at 81 °C" is not
	// one) — and rides SubVerbatim; the plain fallback is a whole key the walk
	// localizes itself.
	sub, subVerbatim := "no limit reported", false
	if t.Warn > 0 {
		sub, subVerbatim = fmt.Sprintf(tr("warns at %s"), celsiusRound(t.Warn)), true
	} else if t.Crit > 0 {
		sub, subVerbatim = fmt.Sprintf(tr("protects at %s"), celsiusRound(t.Crit)), true
	}
	return &widget.Stat{
		Style: "bare", Label: "Processor", Icon: "thermometer",
		Value: celsiusWhole(t.MilliC), Verbatim: true, Unit: "°C",
		Sub: sub, SubVerbatim: subVerbatim,
		Variant: tempVariant(t.Level),
	}
}

func powerTile(tr func(string) string, p *sensors.PowerReading) widget.Widget {
	return &widget.Stat{
		Style: "bare", Label: "Power draw", Icon: "zap",
		Value: watts(p.MicroW), Verbatim: true, Unit: "W",
		Sub: p.Name + " · " + tr("the input"), SubVerbatim: true,
		Variant: "success",
	}
}

func fanTile(tr func(string) string, f *sensors.FanReading) widget.Widget {
	sub := f.Name
	if f.HasDuty {
		sub = f.Name + " · " + fmt.Sprintf(tr("%d%% duty"), f.Duty)
	}
	return &widget.Stat{
		Style: "bare", Label: "Cooling", Icon: "fan-spin",
		Value: rpmGroup(f.RPM), Verbatim: true, Unit: "rpm",
		Sub: sub, SubVerbatim: true, Variant: "success",
	}
}

// warmestTile is the unprofiled grid's second instrument: the warmest reading the
// box has (preferring one with a real limit so its headroom is meaningful).
func warmestTile(tr func(string) string, inv sensors.Inventory) widget.Widget {
	var pick *sensors.TempReading
	for i := range inv.Temps {
		t := &inv.Temps[i]
		if t.CPU || t.Fault {
			continue
		}
		if pick == nil || betterWarmest(t, pick) {
			pick = t
		}
	}
	if pick == nil {
		return nil
	}
	label := "Warmest sensor"
	if strings.Contains(pick.Source, "nvme") {
		label = "Storage"
	}
	sub := pick.Name
	if pick.Warn > 0 {
		sub = pick.Name + " · " + fmt.Sprintf(tr("warns at %s"), celsiusRound(pick.Warn))
	}
	return &widget.Stat{
		Style: "bare", Label: label, Icon: "thermometer",
		Value: celsiusWhole(pick.MilliC), Verbatim: true, Unit: "°C",
		Sub: sub, SubVerbatim: true,
		Variant: tempVariant(pick.Level),
	}
}

// betterWarmest prefers a reading with a real limit, then the hotter one — so the
// warmest tile favours a sensor whose headroom actually means something.
func betterWarmest(a, b *sensors.TempReading) bool {
	if a.HasLimits != b.HasLimits {
		return a.HasLimits
	}
	return a.MilliC > b.MilliC
}

// hardwareTemps is the curated Temperatures section — the profile's picks (or the
// auto-detected set), each a compact headroom bar scaled to its own critical, with
// a quiet note pointing at the rest in the full table.
func hardwareTemps(tr func(string) string, profile *sensors.Profile, inv sensors.Inventory) widget.Widget {
	curated := inv.CuratedTemps()
	if len(curated) == 0 {
		return nil
	}
	rows := make([]widget.Widget, 0, len(curated))
	for i := range curated {
		rows = append(rows, tempRow(&curated[i]))
	}
	// A run of rows: each reading keeps its own air above and below its
	// hairline, so the stack adds none between them.
	sec := &widget.Section{
		Title:    "Temperatures",
		Children: []widget.Widget{&widget.Stack{Flush: true, Children: rows}},
	}
	if profile != nil {
		sec.Sub = "Each bar runs to the point where the hardware would protect itself."
		if cpu := inv.CPUTemp(); cpu != nil && cpu.Crit > 0 {
			sec.Meta = fmt.Sprintf(tr("warns at %s"), celsiusRound(cpu.Warn)) + " · " + fmt.Sprintf(tr("protects at %s"), celsiusRound(cpu.Crit))
			sec.MetaVerbatim = true
		}
	} else {
		sec.Sub = "No profile exists for this board, so this page shows whatever the kernel exposes, under the kernel's own names."
	}
	return sec
}

// tempRow renders one curated temperature as a compact headroom meter: the bar
// fills 0→this sensor's critical, with warn/critical ticks; a reading with no
// limit shows no bar (never a guessed threshold).
func tempRow(t *sensors.TempReading) widget.Widget {
	m := &widget.Meter{Compact: true, Label: t.Name, Verbatim: true, Value: celsiusOne(t.MilliC), Unit: "°C"}
	ceiling := t.Crit
	if ceiling == 0 {
		ceiling = t.Warn
	}
	if ceiling == 0 {
		m.NoTrack = true
		return m
	}
	m.Fill = percent(t.MilliC, ceiling)
	if t.Warn > 0 {
		m.WarnMark = percent(t.Warn, ceiling)
	}
	if t.Crit > 0 {
		m.CritMark = 100
	}
	m.Tone = tempTone(t.Level)
	return m
}

// hardwarePower itemizes the rails the input feeds — sorted by draw, voltage quiet,
// watts the number that moves. The system total (the main rail) is the grid tile
// above, so it is not repeated here. Rendered only where a profile named a main
// rail; an unprofiled box has no honest "total" to itemize against.
func hardwarePower(tr func(string) string, inv sensors.Inventory) widget.Widget {
	if inv.MainPower() == nil {
		return nil
	}
	rails := inv.OtherPowers()
	if len(rails) == 0 {
		return nil
	}
	rows := make([]widget.TableRow, 0, len(rails))
	for _, p := range rails {
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: p.Name},
			{Text: voltage(p)},
			{Text: current(p)},
			{Text: powerText(p.MicroW)},
		}})
	}
	table := &widget.Table{
		Dense: true,
		Columns: []widget.TableColumn{
			{Label: "Rail", Kind: "name"},
			{Label: "Voltage", Kind: "num"},
			{Label: "Current", Kind: "num"},
			{Label: "Power", Kind: "num"},
		},
		Rows: rows,
	}
	meta := fmt.Sprintf(tr("%d rails · sorted by draw"), len(rails))
	if len(rails) == 1 {
		meta = tr("1 rail · sorted by draw")
	}
	return &widget.Section{
		Title: "Power", Hairline: true,
		Meta: meta, MetaVerbatim: true,
		Children: []widget.Widget{table},
	}
}

// hardwareFans keeps a fan tile-strip only when two or more fans actually run —
// one running fan leads the grid above and needs no section. An unpopulated header
// (0 RPM, no alarm) shows quietly as absent.
func hardwareFans(tr func(string) string, inv sensors.Inventory) widget.Widget {
	if inv.RunningFans() < 2 {
		return nil
	}
	others := inv.OtherFans()
	tiles := make([]widget.Widget, 0, len(others))
	for i := range others {
		tiles = append(tiles, fanStripTile(tr, &others[i]))
	}
	return &widget.Section{
		Title: "Fans", Hairline: true,
		Children: []widget.Widget{&widget.Grid{Columns: len(tiles), Children: tiles}},
	}
}

func fanStripTile(tr func(string) string, f *sensors.FanReading) widget.Widget {
	switch f.State {
	case "running":
		sub := f.Name
		if f.HasDuty {
			sub = f.Name + " · " + fmt.Sprintf(tr("%d%% duty"), f.Duty)
		}
		return &widget.Stat{Style: "bare", Label: f.Name, Icon: "fan-spin", Value: rpmGroup(f.RPM), Verbatim: true, Unit: "rpm", Sub: sub, SubVerbatim: true, Variant: "success"}
	case "fault":
		return &widget.Stat{Style: "bare", Label: f.Name, Icon: "fan", Value: "—", Verbatim: true, Sub: "stalled — alarm raised", Variant: "warning"}
	default:
		return &widget.Stat{Style: "bare", Label: f.Name, Icon: "fan", Value: "—", Verbatim: true, Sub: "not connected"}
	}
}

// hardwareFibre renders one card per SFP cage from its module DOM — the optical
// transmit/receive pair leading (dBm, with raw microwatts as the quiet twin), then
// temperature, supply, and laser bias, under a plain link verdict.
func hardwareFibre(inv sensors.Inventory) widget.Widget {
	if len(inv.Fibers) == 0 {
		return nil
	}
	cards := make([]widget.Widget, 0, len(inv.Fibers))
	for i := range inv.Fibers {
		cards = append(cards, fibreCard(&inv.Fibers[i]))
	}
	return &widget.Section{
		Title: "Fiber modules", Hairline: true,
		Meta:     "read from each module's own diagnostics",
		Children: []widget.Widget{&widget.Grid{Columns: 2, Children: cards}},
	}
}

func fibreCard(f *sensors.FiberModule) widget.Widget {
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
	items = append(items, widget.Property{Label: "Temperature", Value: celsiusOne(f.TempMilliC) + " °C", Mono: true})
	if f.HasVcc {
		items = append(items, widget.Property{Label: "Supply", Value: fmt.Sprintf("%.2f V", float64(f.VccMilliV)/1000), Mono: true})
	}
	if f.HasBias {
		items = append(items, widget.Property{Label: "Laser bias", Value: fmt.Sprintf("%d mA", f.BiasMilliA), Mono: true})
	}
	return &widget.Card{
		Title:    "SFP+ module",
		Subtitle: f.Cage,
		Children: []widget.Widget{&widget.Properties{Items: items}},
	}
}

// hardwareReadings is the full instrument panel: every reading the kernel exposes,
// by its own name, in one table banded by kind — a subsection title row spanning the
// columns before each run, the way the firewall listing bands its rules.
func hardwareReadings(inv sensors.Inventory) []widget.Widget {
	var rows []widget.TableRow
	rows = bandReadings(rows, "Temperatures", tempReadingRows(inv))
	rows = bandReadings(rows, "Power", powerReadingRows(inv))
	rows = bandReadings(rows, "Fans", fanReadingRows(inv))
	rows = bandReadings(rows, "Fiber modules", fibreReadingRows(inv))
	if len(rows) == 0 {
		return nil
	}
	section := &widget.Section{
		Title:    "All sensors",
		Hairline: true,
		// Not dense: the value (set right) and its limits (set left) stand side
		// by side and need their cells' inset between them, which four columns
		// have the measure for.
		Children: []widget.Widget{&widget.Table{Columns: readingColumns(), Rows: rows}},
	}
	return []widget.Widget{section}
}

// bandReadings titles a run of rows as a subsection: the kind's name on a header row
// spanning the table, then the rows themselves. An empty kind contributes nothing.
func bandReadings(rows []widget.TableRow, label string, band []widget.TableRow) []widget.TableRow {
	if len(band) == 0 {
		return rows
	}
	band[0].Group = &widget.TableGroup{Label: label}
	return append(rows, band...)
}

func readingColumns() []widget.TableColumn {
	return []widget.TableColumn{
		{Label: "Reading", Kind: "name"},
		{Label: "Source", Kind: "mono"},
		{Label: "Value", Kind: "num"},
		{Label: "Limits", Kind: "comment"},
	}
}

func tempReadingRows(inv sensors.Inventory) []widget.TableRow {
	rows := make([]widget.TableRow, 0, len(inv.Temps))
	for i := range inv.Temps {
		t := &inv.Temps[i]
		value := celsiusOne(t.MilliC) + " °C"
		muted := false
		if t.Fault {
			value, muted = "—", true
		}
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: t.Kernel, LeadIcon: "thermometer", Muted: muted},
			{Text: t.Source, Muted: muted},
			{Text: value, Muted: muted},
			{Text: tempLimitText(t), Muted: muted},
		}})
	}
	return rows
}

func powerReadingRows(inv sensors.Inventory) []widget.TableRow {
	rows := make([]widget.TableRow, 0, len(inv.Powers))
	for _, p := range inv.Powers {
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: p.Name, LeadIcon: "zap"},
			{Text: p.Source},
			{Text: powerText(p.MicroW)},
			{Text: powerLimitText(p)},
		}})
	}
	return rows
}

func fanReadingRows(inv sensors.Inventory) []widget.TableRow {
	rows := make([]widget.TableRow, 0, len(inv.Fans))
	for i := range inv.Fans {
		f := &inv.Fans[i]
		value, detail, muted := rpmGroup(f.RPM)+" rpm", fanLimitText(f), false
		if f.State != "running" {
			value, muted = "—", true
		}
		rows = append(rows, widget.TableRow{Cells: []widget.TableCell{
			{Text: f.Kernel, LeadIcon: "fan", Muted: muted},
			{Text: f.Source, Muted: muted},
			{Text: value, Muted: muted},
			{Text: detail, Muted: muted},
		}})
	}
	return rows
}

func fibreReadingRows(inv sensors.Inventory) []widget.TableRow {
	var rows []widget.TableRow
	for i := range inv.Fibers {
		f := &inv.Fibers[i]
		if f.HasVcc {
			rows = append(rows, fibreRow("VCC", f.Source, fmt.Sprintf("%.2f V", float64(f.VccMilliV)/1000), voltLimit(f.VccMin, f.VccMax)))
		}
		if f.HasTx {
			rows = append(rows, fibreRow("TX_power", f.Source, optical(f.TxMicroW), opticalLimit(f.TxMin, f.TxMax)))
		}
		if f.HasRx {
			rows = append(rows, fibreRow("RX_power", f.Source, optical(f.RxMicroW), opticalLimit(f.RxMin, f.RxMax)))
		}
		if f.HasBias {
			rows = append(rows, fibreRow("bias", f.Source, fmt.Sprintf("%d mA", f.BiasMilliA), biasLimit(f.BiasMax)))
		}
	}
	return rows
}

func fibreRow(name, source, value, limit string) widget.TableRow {
	return widget.TableRow{Cells: []widget.TableCell{
		{Text: name, LeadIcon: "radio"}, {Text: source}, {Text: value}, {Text: limit},
	}}
}

// --- limit + unit formatting ----------------------------------------------

func tempLimitText(t *sensors.TempReading) string {
	switch {
	case t.Fault:
		return "sensor fault"
	case t.HasLimits && t.Kind == "zone":
		return fmt.Sprintf("warn %s · crit %s", celsiusRound(t.Warn), celsiusRound(t.Crit))
	case t.HasLimits:
		return fmt.Sprintf("high %s · crit %s", celsiusRound(t.Warn), celsiusRound(t.Crit))
	default:
		// A driver's sentinel standing in for a limit is no limit, and reads
		// as one.
		return "no limits reported"
	}
}

func powerLimitText(p sensors.PowerReading) string {
	parts := make([]string, 0, 2)
	if p.HasV {
		parts = append(parts, voltage(p))
	}
	if p.HasA {
		parts = append(parts, current(p))
	}
	return strings.Join(parts, " · ")
}

func fanLimitText(f *sensors.FanReading) string {
	switch f.State {
	case "fault":
		return "stalled — alarm"
	case "absent":
		return "not connected"
	case "running":
		if f.HasDuty {
			return fmt.Sprintf("pwm %d%%", f.Duty)
		}
	}
	return ""
}

func voltLimit(min, max int) string {
	if min == 0 && max == 0 {
		return ""
	}
	return fmt.Sprintf("min %.2f · max %.2f", float64(min)/1000, float64(max)/1000)
}

func opticalLimit(min, max int64) string {
	if min == 0 && max == 0 {
		return ""
	}
	return fmt.Sprintf("min %s · max %s", optical(min), optical(max))
}

func biasLimit(max int) string {
	if max == 0 {
		return ""
	}
	return fmt.Sprintf("max %d mA", max)
}

func voltage(p sensors.PowerReading) string {
	if !p.HasV {
		return ""
	}
	return fmt.Sprintf("%.2f V", float64(p.MilliV)/1000)
}

func current(p sensors.PowerReading) string {
	if !p.HasA {
		return ""
	}
	return fmt.Sprintf("%.2f A", float64(p.MilliA)/1000)
}

// celsiusRound renders a milli-°C as a whole degree ("85 °C"); celsiusOne keeps
// one decimal ("47.0 °C").
func celsiusRound(milliC int) string { return fmt.Sprintf("%d °C", (milliC+500)/1000) }

// celsiusWhole is the rounded degrees with no unit — a tile carries its "°C" in the
// Stat's own unit slot, so the value must not repeat it.
func celsiusWhole(milliC int) string { return fmt.Sprintf("%d", (milliC+500)/1000) }

func celsiusOne(milliC int) string { return fmt.Sprintf("%.1f", float64(milliC)/1000) }

func watts(microW int64) string { return fmt.Sprintf("%.1f", float64(microW)/1e6) }

// powerText renders a channel's draw in the unit that keeps its figure legible —
// milliwatts for the sub-watt channels a GPU reports, watts for a rail.
func powerText(microW int64) string {
	if microW != 0 && microW > -1_000_000 && microW < 1_000_000 {
		return fmt.Sprintf("%d mW", microW/1000)
	}
	return fmt.Sprintf("%.1f W", float64(microW)/1e6)
}

// optical renders a microwatt reading the way a module reports it — plain
// microwatts under a milliwatt, milliwatts above.
func optical(microW int64) string {
	if microW >= 1000 {
		return fmt.Sprintf("%.2f mW", float64(microW)/1000)
	}
	return fmt.Sprintf("%d µW", microW)
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
		return "< −40 dBm"
	}
	return fmt.Sprintf("%.1f dBm", 10*math.Log10(float64(microW)/1000))
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

func tempTone(level string) string {
	switch level {
	case "warn":
		return "warning"
	case "critical":
		return "danger"
	default:
		return "success"
	}
}

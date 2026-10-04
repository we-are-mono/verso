// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Inventory is the box's complete sensor enumeration — every temperature, power
// rail, fan, and fibre module the kernel exposes, resolved through the board
// profile where one exists. It is the one source of truth for the Hardware page
// and the overview's headline readings alike. Readings carry raw units
// (milli-°C, milli-volt, milli-amp, micro-watt, RPM); the presentation layer
// formats them, so the resolution contract stays unit-tested against sysfs.
type Inventory struct {
	Temps  []TempReading
	Powers []PowerReading
	Fans   []FanReading
	Fibers []FiberModule
}

// TempReading is one temperature the box measures — a thermal zone or a hwmon
// channel. Name is the display name (the profile's when curated, else the kernel's);
// Kernel is the verbatim name the full table prints; Source is the chip locator.
// Warn/Crit are the reading's own limits in milli-°C (0 when unset); Level grades
// it against them ("nominal"/"warn"/"critical", "" when it has no limits and must
// not be graded against a guess). Fault marks an invalid reading — a disconnected
// remote diode reads 0 and renders "—", never a real 0.
type TempReading struct {
	Name              string
	Kernel            string
	Source            string
	Kind              string // "zone" | "hwmon"
	MilliC            int
	Warn              int
	Crit              int
	HasLimits         bool
	SentinelDiscarded bool // limit files existed but held out-of-range sentinels
	Level             string
	Fault             bool
	CPU               bool // the headline processor pick lifted to the vitals grid
	Curated           bool // shown in the curated Temperatures section
	Rank              int  // order within the curated section

	zoneType string
	ofNode   string
	channel  int
	chip     string
	instance string // a hwmon's own sysfs dir — a stable unique key across a read
}

// PowerReading is one power channel. A profiled board names it and flags the one
// input rail Main (the honest system total). Voltage and current come from the
// same chip so a rail reads as V · A · W.
type PowerReading struct {
	Name   string
	Kernel string
	Source string
	MilliV int
	MilliA int
	MicroW int64
	HasV   bool
	HasA   bool
	Main   bool
}

// FanReading is one fan channel. Duty is the pwm duty as a percent (0–255 scaled);
// State reads the channel's presence — "running" (RPM > 0), "fault" (stalled with
// an alarm set), or "absent" (an unpopulated header, RPM 0 and no alarm).
type FanReading struct {
	Name    string
	Kernel  string
	Source  string
	RPM     int
	Duty    int
	HasDuty bool
	State   string // "running" | "fault" | "absent"
	Main    bool
}

// FiberModule is one SFP/SFP+ cage's diagnostics, read from its module DOM through
// hwmon (VCC as an in channel, temperature, transmit/receive optical power, laser
// bias current) with the module's own min/max/crit limits. RxHasLight is the
// loss-of-signal verdict: receive power above the module's own floor.
type FiberModule struct {
	Cage   string
	Source string

	TempMilliC int

	VccMilliV int
	VccMin    int
	VccMax    int
	HasVcc    bool

	BiasMilliA int
	BiasMax    int
	HasBias    bool

	TxMicroW int64
	TxMin    int64
	TxMax    int64
	HasTx    bool

	RxMicroW   int64
	RxMin      int64
	RxMax      int64
	HasRx      bool
	RxHasLight bool
}

// ResolveAll reads the box's sensors and enumerates all of them, resolving names
// and headline picks through the profile when one matches. It reads local sysfs
// only, so it needs no session and blocks on nothing — the Hardware page calls it
// fresh on each load, and the overview on each render and stream tick.
func ResolveAll(p *Profile, r *Reader) Inventory {
	zones := r.Zones()
	hwmons := r.Hwmons()
	amb := ambiguousNames(hwmons)
	return Inventory{
		Temps:  resolveTemps(p, zones, hwmons, amb),
		Powers: resolvePowers(p, hwmons, amb),
		Fans:   resolveFans(p, hwmons, amb),
		Fibers: resolveFibers(hwmons),
	}
}

// ambiguousNames is the set of driver names carried by more than one hwmon — the
// ones whose readings need a bus address to tell apart (a board's eight ina234 or
// two NVMe drives). A chip that is the only one of its name (a lone k10temp) shows
// its bare name, the way `sensors` does; the locator is disambiguation, not chrome.
func ambiguousNames(hwmons []Hwmon) map[string]bool {
	counts := map[string]int{}
	for _, h := range hwmons {
		counts[h.Name]++
	}
	amb := map[string]bool{}
	for name, n := range counts {
		if n > 1 {
			amb[name] = true
		}
	}
	return amb
}

// resolveTemps builds every temperature reading: each thermal zone, then each
// hwmon temperature channel whose sensor is not already a zone twin (deduped by
// the shared of_node, or the zone-type/hwmon-name pairing the LS1046A's TMU uses).
// It then lifts the profile's curated names and headline pick onto the matching
// readings; unprofiled, it curates each zone and each chip's primary channel.
func resolveTemps(p *Profile, zones []Zone, hwmons []Hwmon, amb map[string]bool) []TempReading {
	var out []TempReading
	for _, z := range zones {
		out = append(out, tempFromZoneReading(z))
	}
	for _, h := range hwmons {
		for _, ch := range slices.Sorted(maps.Keys(h.Temp)) {
			if zoneTwin(zones, h, ch) {
				continue
			}
			out = append(out, tempFromHwmon(h, ch, amb[h.Name]))
		}
	}
	if p != nil {
		curateProfileTemps(out, p)
	} else {
		curateGenericTemps(out)
	}
	return out
}

// tempFromZoneReading turns a thermal zone into a reading, taking its warn/critical
// from the trip points it carries.
func tempFromZoneReading(z Zone) TempReading {
	warn, crit := tripLimits(z.Trips)
	t := TempReading{
		Name: z.Type, Kernel: z.Type, Source: "thermal zone", Kind: "zone",
		MilliC: z.MilliC, Warn: warn, Crit: crit,
		zoneType: z.Type, ofNode: z.OfNode,
	}
	t.HasLimits = warn > 0 || crit > 0
	t.Level = gradeTemp(z.MilliC, warn, crit)
	return t
}

// tempFromHwmon turns a hwmon temperature channel into a reading, reading its
// label (the verbatim name), max/crit limits, and fault flag from the chip.
func tempFromHwmon(h Hwmon, ch int, ambiguous bool) TempReading {
	kernel := readStr(filepath.Join(h.Dir, tempAttr(ch, "label")))
	if kernel == "" {
		kernel = "temp" + strconv.Itoa(ch)
	}
	warn, warnSeen := saneTempLimit(readIntOK(filepath.Join(h.Dir, tempAttr(ch, "max"))))
	crit, critSeen := saneTempLimit(readIntOK(filepath.Join(h.Dir, tempAttr(ch, "crit"))))
	fault := readFlag(filepath.Join(h.Dir, tempAttr(ch, "fault")))
	t := TempReading{
		Name: kernel, Kernel: kernel, Source: hwmonSource(h, ambiguous), Kind: "hwmon",
		MilliC: h.Temp[ch], Warn: warn, Crit: crit, Fault: fault,
		ofNode: h.OfNode, channel: ch, chip: h.Name, instance: h.Dir,
	}
	t.HasLimits = warn > 0 || crit > 0
	// A remote diode reads 0 when unconnected; the fault flag, not the value, is
	// the truth, so a faulted channel is graded neutral and rendered "—".
	if !fault {
		t.Level = gradeTemp(h.Temp[ch], warn, crit)
	}
	limitFilesSeen := warnSeen || critSeen
	t.SentinelDiscarded = limitFilesSeen && !t.HasLimits
	return t
}

// curateProfileTemps lifts the profile's thermal entries onto the matching
// readings: each entry's chosen name, its place in the curated section, and the
// one entry flagged the CPU. An entry keyed with a :tempN selector matches a
// specific hwmon channel; a bare zone type matches a zone. A profile whose CPU
// pick is absent or unreadable falls back to the generic detection an unprofiled
// board gets, so the grid never loses its temperature to a stale profile.
func curateProfileTemps(temps []TempReading, p *Profile) {
	cpu := false
	for rank, e := range p.Thermal {
		i := matchThermalEntry(temps, e.Path)
		if i < 0 {
			continue
		}
		temps[i].Name = e.Name
		if e.CPU {
			temps[i].CPU, cpu = true, true
			continue // the CPU pick leads the grid, not the curated list
		}
		temps[i].Curated = true
		temps[i].Rank = rank
	}
	if !cpu {
		if i := genericCPUIndex(temps); i >= 0 {
			temps[i].CPU, temps[i].Curated = true, false
		}
	}
}

// curateGenericTemps curates an unprofiled board: every zone, and each chip's
// primary (lowest-index) channel, except the auto-detected CPU pick — which leads
// the grid. Secondary channels (a drive's per-die sensors) stay in the full table.
func curateGenericTemps(temps []TempReading) {
	cpu := genericCPUIndex(temps)
	if cpu >= 0 {
		temps[cpu].CPU = true
	}
	// Group by the chip instance's own sysfs dir — a key that is unique per hwmon
	// even when two identical NVMe drives share a driver name and a bare source —
	// keeping the lowest channel as the chip's primary.
	primary := map[string]int{}
	for i := range temps {
		if temps[i].Kind != "hwmon" {
			continue
		}
		key := temps[i].instance
		if j, ok := primary[key]; !ok || temps[i].channel < temps[j].channel {
			primary[key] = i
		}
	}
	rank := 0
	for i := range temps {
		if temps[i].CPU {
			continue
		}
		isPrimary := temps[i].Kind == "zone"
		if temps[i].Kind == "hwmon" {
			isPrimary = primary[temps[i].instance] == i
		}
		if isPrimary {
			temps[i].Curated = true
			temps[i].Rank = rank
			rank++
		}
	}
}

// resolvePowers enumerates every hwmon power channel (excluding the optical power
// an SFP module reports — that belongs to its fibre card), pairing each with its
// voltage and current from the same chip. A profile names the rails and flags the
// input rail Main; unprofiled, the channels still list in the full table.
func resolvePowers(p *Profile, hwmons []Hwmon, amb map[string]bool) []PowerReading {
	var out []PowerReading
	for _, h := range hwmons {
		if isFiber(h.Name) {
			continue
		}
		for _, ch := range slices.Sorted(maps.Keys(h.Power)) {
			kernel := readStr(filepath.Join(h.Dir, chanAttr("power", ch, "label")))
			if kernel == "" {
				kernel = "power" + strconv.Itoa(ch)
			}
			pr := PowerReading{Kernel: kernel, Source: hwmonSource(h, amb[h.Name]), MicroW: h.Power[ch]}
			if name, main, ok := matchPowerEntry(p, h); ok {
				pr.Name, pr.Main = name, main
			}
			if pr.Name == "" {
				pr.Name = pr.Kernel
			}
			if mv, ok := readIntOK(filepath.Join(h.Dir, chanAttr("in", ch, "input"))); ok {
				pr.MilliV, pr.HasV = mv, true
			}
			if ma, ok := readIntOK(filepath.Join(h.Dir, chanAttr("curr", ch, "input"))); ok {
				pr.MilliA, pr.HasA = ma, true
			}
			out = append(out, pr)
		}
	}
	return out
}

// matchPowerEntry resolves a profile power entry for a chip by its of_node tail —
// each INA rail is its own single-channel hwmon.
func matchPowerEntry(p *Profile, h Hwmon) (name string, main, ok bool) {
	if p == nil {
		return "", false, false
	}
	for _, e := range p.Power {
		if suffixMatch(h.OfNode, e.Path) {
			return e.Name, e.Main, true
		}
	}
	return "", false, false
}

// resolveFans enumerates every hwmon fan channel with its pwm duty and presence.
// A profile names the fans and flags the main one; unprofiled, they keep kernel
// names. The page decides layout from how many are actually running.
func resolveFans(p *Profile, hwmons []Hwmon, amb map[string]bool) []FanReading {
	var out []FanReading
	for _, h := range hwmons {
		for _, ch := range slices.Sorted(maps.Keys(h.Fan)) {
			rpm := h.Fan[ch]
			fr := FanReading{
				Kernel: "fan" + strconv.Itoa(ch), Name: "fan" + strconv.Itoa(ch),
				Source: hwmonSource(h, amb[h.Name]), RPM: rpm,
			}
			if duty, ok := readIntOK(filepath.Join(h.Dir, "pwm"+strconv.Itoa(ch))); ok {
				fr.Duty, fr.HasDuty = (duty*100+127)/255, true
			}
			fault := readFlag(filepath.Join(h.Dir, chanAttr("fan", ch, "fault"))) ||
				readFlag(filepath.Join(h.Dir, chanAttr("fan", ch, "alarm")))
			switch {
			case rpm > 0:
				fr.State = "running"
			case fault:
				fr.State = "fault"
			default:
				fr.State = "absent"
			}
			if name, main, ok := matchFanEntry(p, h, ch); ok {
				fr.Name, fr.Main = name, main
			}
			out = append(out, fr)
		}
	}
	return out
}

// resolveFibers builds a card per SFP cage from its module DOM. The cages are ISA
// hwmon with no of_node, so they match by name ("sfp_xfi0" → cage "xfi0").
func resolveFibers(hwmons []Hwmon) []FiberModule {
	var out []FiberModule
	for _, h := range hwmons {
		if !isFiber(h.Name) {
			continue
		}
		out = append(out, fiberFrom(h))
	}
	return out
}

func fiberFrom(h Hwmon) FiberModule {
	// A fibre cage's source is always "sfp · <cage>" — the cage name already
	// disambiguates, so the driver-name ambiguity rule does not apply.
	f := FiberModule{Cage: fiberCage(h.Name), Source: hwmonSource(h, false), TempMilliC: h.Temp[1]}

	vin := firstKey(h)
	if mv, ok := readIntOK(filepath.Join(h.Dir, chanAttr("in", vin, "input"))); ok {
		f.VccMilliV, f.HasVcc = mv, true
		f.VccMin, _ = readIntOK(filepath.Join(h.Dir, chanAttr("in", vin, "min")))
		f.VccMax, _ = readIntOK(filepath.Join(h.Dir, chanAttr("in", vin, "max")))
	}
	if ma, ok := readIntOK(filepath.Join(h.Dir, chanAttr("curr", 1, "input"))); ok {
		f.BiasMilliA, f.HasBias = ma, true
		f.BiasMax, _ = readIntOK(filepath.Join(h.Dir, chanAttr("curr", 1, "max")))
	}
	if uw, ok := readInt64OK(filepath.Join(h.Dir, chanAttr("power", 1, "input"))); ok {
		f.TxMicroW, f.HasTx = uw, true
		f.TxMin, _ = readInt64OK(filepath.Join(h.Dir, chanAttr("power", 1, "min")))
		f.TxMax, _ = readInt64OK(filepath.Join(h.Dir, chanAttr("power", 1, "max")))
	}
	if uw, ok := readInt64OK(filepath.Join(h.Dir, chanAttr("power", 2, "input"))); ok {
		f.RxMicroW, f.HasRx = uw, true
		f.RxMin, _ = readInt64OK(filepath.Join(h.Dir, chanAttr("power", 2, "min")))
		f.RxMax, _ = readInt64OK(filepath.Join(h.Dir, chanAttr("power", 2, "max")))
		// Receive power under a receiver-sensitivity floor (~−20 dBm) is loss of
		// signal — the honest verdict a cage gives from its own DOM, without
		// asking the interface its link speed.
		f.RxHasLight = uw >= rxLightFloorMicroW
	}
	return f
}

// rxLightFloorMicroW is ~−20 dBm — under it a 10G receiver has no usable signal.
const rxLightFloorMicroW = 10

// --- convenience selectors the page composes with -------------------------

// CPUTemp returns the reading lifted onto the vitals grid, or nil.
func (inv Inventory) CPUTemp() *TempReading {
	for i := range inv.Temps {
		if inv.Temps[i].CPU {
			return &inv.Temps[i]
		}
	}
	return nil
}

// CuratedTemps returns the curated Temperatures section rows, in profile order.
func (inv Inventory) CuratedTemps() []TempReading {
	var out []TempReading
	for _, t := range inv.Temps {
		if t.Curated {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

// MainPower returns the input rail (the system total), or nil when unprofiled.
func (inv Inventory) MainPower() *PowerReading {
	for i := range inv.Powers {
		if inv.Powers[i].Main {
			return &inv.Powers[i]
		}
	}
	return nil
}

// OtherPowers returns the non-main rails sorted by draw, descending — the Power
// section's itemization of what the input feeds.
func (inv Inventory) OtherPowers() []PowerReading {
	var out []PowerReading
	for _, p := range inv.Powers {
		if !p.Main {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MicroW > out[j].MicroW })
	return out
}

// RunningFans counts the fan channels actually spinning — the collapse rule's
// input (one running fan drops the Fans section; two or more keep it).
func (inv Inventory) RunningFans() int {
	n := 0
	for _, f := range inv.Fans {
		if f.State == "running" {
			n++
		}
	}
	return n
}

// MainFan returns the fan lifted onto the grid — the profile's main, else the
// fastest running channel.
func (inv Inventory) MainFan() *FanReading {
	var pick *FanReading
	for i := range inv.Fans {
		if inv.Fans[i].Main {
			return &inv.Fans[i]
		}
		if inv.Fans[i].State == "running" && (pick == nil || inv.Fans[i].RPM > pick.RPM) {
			pick = &inv.Fans[i]
		}
	}
	return pick
}

// OtherFans returns every fan except the one lifted onto the grid — the tile-strip's
// contents when a board runs more than one, so the grid fan is never shown twice.
func (inv Inventory) OtherFans() []FanReading {
	main := inv.MainFan()
	var out []FanReading
	for _, f := range inv.Fans {
		if main != nil && f.Source == main.Source && f.Kernel == main.Kernel {
			continue
		}
		out = append(out, f)
	}
	return out
}

// --- resolution helpers ---------------------------------------------------

// gradeTemp grades a reading against its own limits, never a guessed one: no
// limits means no grade (a neutral dot, no bar).
func gradeTemp(milliC, warn, crit int) string {
	switch {
	case crit > 0 && milliC >= crit:
		return "critical"
	case warn > 0 && milliC >= warn:
		return "warn"
	case warn > 0 || crit > 0:
		return "nominal"
	default:
		return ""
	}
}

// saneTempLimit discards a hwmon sentinel — an NVMe drive advertises a high of
// +65261.8 °C and a low of −273.1 °C to mean "unset", and a bare 0 is not a real
// thermal limit either. present reports whether the file existed at all, so the
// caller can tell "no limits" from "limits discarded".
func saneTempLimit(v int, ok bool) (int, bool) {
	if !ok || v == 0 || v < -100000 || v > 250000 {
		return 0, ok
	}
	return v, true
}

// tripLimits reduces a zone's trip points to a warn (the lowest of passive/hot/
// active) and a critical.
func tripLimits(trips []Trip) (warn, crit int) {
	for _, t := range trips {
		switch t.Type {
		case "critical":
			crit = t.MilliC
		case "hot", "passive", "active":
			if warn == 0 || t.MilliC < warn {
				warn = t.MilliC
			}
		}
	}
	return warn, crit
}

// zoneTwin reports whether a hwmon temperature channel mirrors a thermal zone —
// the same sensor exposed twice. They share the of_node, and the LS1046A's TMU
// also pairs a zone type ("cluster-thermal") with a hwmon name ("cluster_thermal").
func zoneTwin(zones []Zone, h Hwmon, ch int) bool {
	label := readStr(filepath.Join(h.Dir, tempAttr(ch, "label")))
	for _, z := range zones {
		if h.OfNode != "" && z.OfNode == h.OfNode {
			return true
		}
		if normalizeThermal(z.Type) != "" &&
			(normalizeThermal(z.Type) == normalizeThermal(h.Name) ||
				normalizeThermal(z.Type) == normalizeThermal(label)) {
			return true
		}
	}
	return false
}

// normalizeThermal folds a thermal name to compare a zone type against a hwmon
// name across the '-'/'_' the TMU spells them with.
func normalizeThermal(s string) string {
	return strings.ReplaceAll(s, "_", "-")
}

// matchThermalEntry finds the reading a profile thermal key names — a zone by
// type, or a hwmon channel by its of_node tail and optional :tempN selector.
func matchThermalEntry(temps []TempReading, key string) int {
	base, ch, hasCh := splitChannelSelector(key)
	if !strings.Contains(base, "/") && !hasCh {
		for i := range temps {
			if temps[i].Kind == "zone" && temps[i].zoneType == base {
				return i
			}
		}
		return -1
	}
	for i := range temps {
		if temps[i].Kind != "hwmon" || !suffixMatch(temps[i].ofNode, base) {
			continue
		}
		if hasCh {
			if temps[i].channel == ch {
				return i
			}
			continue
		}
		return i
	}
	return -1
}

// splitChannelSelector separates a thermal key's optional :tempN channel selector
// from its of_node tail. A key without one keeps its whole self.
func splitChannelSelector(key string) (base string, ch int, has bool) {
	i := strings.LastIndex(key, ":")
	if i < 0 {
		return key, 0, false
	}
	sel := key[i+1:]
	if n, err := strconv.Atoi(strings.TrimPrefix(sel, "temp")); err == nil && strings.HasPrefix(sel, "temp") {
		return key[:i], n, true
	}
	return key, 0, false
}

// genericCPUIndex auto-detects the CPU temperature's index: a CPU-ish zone first,
// then a coretemp/k10temp hwmon's hottest channel, then the hottest zone.
func genericCPUIndex(temps []TempReading) int {
	for i := range temps {
		if temps[i].Kind == "zone" && looksLikeCPU(temps[i].zoneType) {
			return i
		}
	}
	best := -1
	for i := range temps {
		if temps[i].Kind == "hwmon" && (temps[i].chip == "coretemp" || temps[i].chip == "k10temp") {
			if best < 0 || temps[i].MilliC > temps[best].MilliC {
				best = i
			}
		}
	}
	if best >= 0 {
		return best
	}
	for i := range temps {
		if temps[i].Kind == "zone" && (best < 0 || temps[i].MilliC > temps[best].MilliC) {
			best = i
		}
	}
	return best
}

// matchFanEntry resolves a profile fan entry for a channel: each key points at a
// controller child (…/fan-controller@2e/fan@0), so the parent of_node identifies
// the hwmon and the child index the channel (fan@0 → fan1).
func matchFanEntry(p *Profile, h Hwmon, ch int) (name string, main, ok bool) {
	if p == nil {
		return "", false, false
	}
	for _, e := range p.Fans {
		parent, child := splitLastSeg(e.Path)
		idx, valid := indexAfterAt(child)
		if !valid || idx+1 != ch || !suffixMatch(h.OfNode, parent) {
			continue
		}
		return e.Name, e.Main, true
	}
	return "", false, false
}

// hwmonSource is the mono locator the full table prints for a chip. A chip whose
// driver name is unique on the box reads as the bare name (`sensors`' own style:
// "k10temp", "amdgpu"). Only when a name is shared by several chips does the bus
// address disambiguate them — "tmp431 · 5-4c", "ina234 · 12-40", "nvme · 05:00" —
// taken from anywhere in the device chain, not just the leaf (an NVMe leaf is
// "nvme0", its bus a PCI address one level up). A fibre cage names itself.
func hwmonSource(h Hwmon, ambiguous bool) string {
	if isFiber(h.Name) {
		return "sfp · " + fiberCage(h.Name)
	}
	if ambiguous {
		if loc := deviceLocator(h.Dir); loc != "" {
			return h.Name + " · " + loc
		}
	}
	if h.Name != "" {
		return h.Name
	}
	return "sensor"
}

// deviceLocator resolves a hwmon's device chain and returns the innermost bus
// address in it — an i2c "5-4c" or a PCI "05:00", trimmed the way sensors prints
// it. Empty when the chain carries neither (a bare platform device).
func deviceLocator(dir string) string {
	target, err := filepath.EvalSymlinks(filepath.Join(dir, "device"))
	if err != nil {
		return ""
	}
	parts := strings.Split(target, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if loc := busLocator(parts[i]); loc != "" {
			return loc
		}
	}
	return ""
}

// busLocator trims a device's bus-address basename to the short form `sensors`
// prints: an i2c "5-004c" to "5-4c", a PCI "0000:05:00.0" to "05:00".
func busLocator(base string) string {
	if base == "" {
		return ""
	}
	if i := strings.IndexByte(base, '-'); i > 0 && allDigits(base[:i]) {
		addr := strings.TrimLeft(base[i+1:], "0")
		if addr == "" {
			addr = "0"
		}
		return base[:i] + "-" + addr
	}
	if parts := strings.Split(base, ":"); len(parts) == 3 {
		return parts[1] + ":" + strings.SplitN(parts[2], ".", 2)[0]
	}
	return ""
}

func isFiber(name string) bool { return strings.HasPrefix(name, "sfp") }

func fiberCage(name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, "sfp"), "_")
}

func tempAttr(ch int, suffix string) string { return chanAttr("temp", ch, suffix) }

func chanAttr(kind string, ch int, suffix string) string {
	return kind + strconv.Itoa(ch) + "_" + suffix
}

// firstKey returns the lowest in-channel index a fibre chip exposes for its VCC.
func firstKey(h Hwmon) int {
	best, ok := 0, false
	matches, _ := filepath.Glob(filepath.Join(h.Dir, "in*_input"))
	for _, m := range matches {
		if n, good := channelIndex(filepath.Base(m), "in"); good && (!ok || n < best) {
			best, ok = n, true
		}
	}
	return best
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

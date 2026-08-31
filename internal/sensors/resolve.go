// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"strconv"
	"strings"
)

// Facts are the resolved home-dashboard sensor readings. A nil pointer means the
// box exposes no such reading and the row is hidden — never a zero shown as real.
type Facts struct {
	CPUTemp *Temp
	Fan     *Fan
	Power   *Power

	PowerCount   int // power channels the box exposes (for the summary)
	ThermalCount int // thermal sources the box exposes
}

// Temp is the CPU temperature with its trip-derived status and a tone level
// ("success"|"warning"|"danger") for the dot.
type Temp struct {
	MilliC int
	Status string
	Level  string
}

// Fan is the headline fan speed in RPM (the fastest running channel).
type Fan struct {
	RPM int
}

// Power is the headline draw in micro-watts (the board input rail).
type Power struct {
	MicroW int64
}

// Resolve reads the box's sensors and produces the home-dashboard facts. With a
// profile, sensors are keyed by their of_node path tails (power and thermal
// matched directly, a fan's path pointing at its controller's channel child);
// without one, CPU temp and fan are auto-detected and power is left unread — an
// unprofiled board can't say which of many rails is the headline draw.
func Resolve(p *Profile, r *Reader) Facts {
	zones := r.Zones()
	hwmons := r.Hwmons()

	var f Facts
	for _, h := range hwmons {
		f.PowerCount += len(h.Power)
	}
	f.ThermalCount = len(zones)
	if f.ThermalCount == 0 {
		for _, h := range hwmons {
			f.ThermalCount += len(h.Temp)
		}
	}

	if p != nil {
		f.CPUTemp = profileCPUTemp(p, zones)
		f.Fan = profileFan(p, hwmons)
		f.Power = profilePower(p, hwmons)
	} else {
		f.CPUTemp = genericCPUTemp(zones, hwmons)
		f.Fan = genericFan(hwmons)
		// No generic power: hidden.
	}
	return f
}

// profileCPUTemp resolves the thermal entry flagged cpu:true, matching a zone by
// its type (the key's last path segment).
func profileCPUTemp(p *Profile, zones []Zone) *Temp {
	for _, e := range p.Thermal {
		if !e.CPU {
			continue
		}
		if z, ok := matchZone(zones, e.Path); ok {
			return tempFromZone(z)
		}
	}
	return nil
}

// profileFan resolves the profile's fan entries and returns the fastest running
// channel. Each key points at a controller child (…/fan-controller@2e/fan@0):
// the parent of_node identifies the hwmon, the child's index the channel.
func profileFan(p *Profile, hwmons []Hwmon) *Fan {
	best := -1
	for _, e := range p.Fans {
		parent, child := splitLastSeg(e.Path)
		ch, ok := indexAfterAt(child)
		if !ok {
			continue
		}
		h, ok := matchHwmon(hwmons, parent)
		if !ok {
			continue
		}
		if rpm, ok := h.Fan[ch+1]; ok && rpm > best { // fan@0 → fan1_input
			best = rpm
		}
	}
	if best < 0 {
		return nil
	}
	return &Fan{RPM: best}
}

// profilePower resolves the power entry flagged main:true — the board input rail,
// the honest total draw.
func profilePower(p *Profile, hwmons []Hwmon) *Power {
	for _, e := range p.Power {
		if !e.Main {
			continue
		}
		if h, ok := matchHwmon(hwmons, e.Path); ok {
			if uw, ok := firstChannel64(h.Power); ok {
				return &Power{MicroW: uw}
			}
		}
	}
	return nil
}

// genericCPUTemp auto-detects a CPU temperature: a CPU-ish thermal zone first,
// then a coretemp/k10temp hwmon, then the hottest zone as a last resort.
func genericCPUTemp(zones []Zone, hwmons []Hwmon) *Temp {
	for _, z := range zones {
		if looksLikeCPU(z.Type) {
			return tempFromZone(z)
		}
	}
	for _, h := range hwmons {
		if h.Name == "coretemp" || h.Name == "k10temp" {
			if mc, ok := maxChannel(h.Temp); ok {
				status, level := classify(mc, nil)
				return &Temp{MilliC: mc, Status: status, Level: level}
			}
		}
	}
	hottest := -1
	var pick *Zone
	for i := range zones {
		if zones[i].MilliC > hottest {
			hottest, pick = zones[i].MilliC, &zones[i]
		}
	}
	if pick == nil {
		return nil
	}
	return tempFromZone(*pick)
}

// genericFan returns the fastest running fan across all hwmon chips.
func genericFan(hwmons []Hwmon) *Fan {
	best := -1
	for _, h := range hwmons {
		if rpm, ok := maxChannel(h.Fan); ok && rpm > best {
			best = rpm
		}
	}
	if best <= 0 {
		return nil
	}
	return &Fan{RPM: best}
}

func tempFromZone(z Zone) *Temp {
	status, level := classify(z.MilliC, z.Trips)
	return &Temp{MilliC: z.MilliC, Status: status, Level: level}
}

// classify grades a temperature against its trips (or fixed fallbacks): below the
// warn trip is Normal, at/above it Warm, at/above critical Critical.
func classify(milliC int, trips []Trip) (status, level string) {
	crit, warn := 0, 0
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
	if crit == 0 {
		crit = 90000
	}
	if warn == 0 {
		warn = 75000
	}
	switch {
	case milliC >= crit:
		return "Critical", "danger"
	case milliC >= warn:
		return "Warm", "warning"
	default:
		return "Normal", "success"
	}
}

func looksLikeCPU(t string) bool {
	t = strings.ToLower(t)
	for _, k := range []string{"x86_pkg", "cpu", "cluster", "coretemp", "package", "tctl"} {
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}

// matchZone finds a zone whose type equals the key's last path segment.
func matchZone(zones []Zone, key string) (Zone, bool) {
	_, last := splitLastSeg(key)
	if last == "" {
		last = key
	}
	for _, z := range zones {
		if z.Type == last {
			return z, true
		}
	}
	return Zone{}, false
}

// matchHwmon finds the hwmon whose of_node path ends with key.
func matchHwmon(hwmons []Hwmon, key string) (Hwmon, bool) {
	for _, h := range hwmons {
		if suffixMatch(h.OfNode, key) {
			return h, true
		}
	}
	return Hwmon{}, false
}

// suffixMatch reports whether an of_node path ends at the profile key — the whole
// path, or a trailing run of its segments.
func suffixMatch(ofNode, key string) bool {
	return ofNode != "" && (ofNode == key || strings.HasSuffix(ofNode, "/"+key))
}

// splitLastSeg splits a "/"-path into its parent and final segment.
func splitLastSeg(path string) (parent, last string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

// indexAfterAt parses the unit-address of a node segment ("fan@0" → 0), which for
// a controller's channel child is its channel number.
func indexAfterAt(seg string) (int, bool) {
	i := strings.LastIndex(seg, "@")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(seg[i+1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

// maxChannel returns the highest reading across a channel map.
func maxChannel(m map[int]int) (int, bool) {
	best, ok := 0, false
	for _, v := range m {
		if !ok || v > best {
			best, ok = v, true
		}
	}
	return best, ok
}

// firstChannel64 returns the lowest-indexed channel's value (an INA power hwmon
// has a single power input).
func firstChannel64(m map[int]int64) (int64, bool) {
	idx, ok := 0, false
	for k := range m {
		if !ok || k < idx {
			idx, ok = k, true
		}
	}
	if !ok {
		return 0, false
	}
	return m[idx], true
}

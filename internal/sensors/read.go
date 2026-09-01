// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Reader reads the kernel's sensor sysfs. Root defaults to "/sys"; tests point it
// at a fixture tree. It is stateless — each Read re-scans, because hwmonN /
// thermal_zoneN numbering is not stable across boots.
type Reader struct {
	Root string // default "/sys"
}

// Zone is one thermal zone: its stable type (the match key), the current reading
// in milli-°C, its trip points (which give warn/critical without a profile), and
// the device-tree node it reads (OfNode) — the shared of_node lets the full
// enumeration drop a hwmon twin that mirrors the same sensor.
type Zone struct {
	Type   string
	MilliC int
	Trips  []Trip
	OfNode string
}

// Trip is one thermal trip point: its kind ("passive"/"hot"/"critical") and the
// milli-°C at which it fires.
type Trip struct {
	Type   string
	MilliC int
}

// Hwmon is one hwmon chip: its driver Name, its device-tree node path (OfNode,
// empty on non-DT systems like x86), the sysfs directory it was read from (Dir,
// so the full enumeration can pull a channel's auxiliary attributes — limits,
// fault flags, labels, voltage, current, pwm — without re-globbing), and its
// readings per channel index — temps in milli-°C, fans in RPM, power in micro-W.
type Hwmon struct {
	Name   string
	OfNode string
	Dir    string
	Temp   map[int]int
	Fan    map[int]int
	Power  map[int]int64
}

func (r *Reader) root() string {
	if r.Root == "" {
		return "/sys"
	}
	return r.Root
}

// Zones reads every /sys/class/thermal/thermal_zone*.
func (r *Reader) Zones() []Zone {
	dirs, _ := filepath.Glob(filepath.Join(r.root(), "class/thermal/thermal_zone*"))
	sort.Strings(dirs)
	out := make([]Zone, 0, len(dirs))
	for _, d := range dirs {
		typ := readStr(filepath.Join(d, "type"))
		if typ == "" {
			continue
		}
		out = append(out, Zone{
			Type:   typ,
			MilliC: readInt(filepath.Join(d, "temp")),
			Trips:  readTrips(d),
			OfNode: ofNodePath(filepath.Join(d, "device", "of_node")),
		})
	}
	return out
}

// readTrips gathers a zone's trip_point_N_{type,temp} pairs.
func readTrips(zoneDir string) []Trip {
	temps, _ := filepath.Glob(filepath.Join(zoneDir, "trip_point_*_temp"))
	sort.Strings(temps)
	out := make([]Trip, 0, len(temps))
	for _, tf := range temps {
		typF := strings.TrimSuffix(tf, "_temp") + "_type"
		typ := readStr(typF)
		if typ == "" {
			continue
		}
		out = append(out, Trip{Type: typ, MilliC: readInt(tf)})
	}
	return out
}

// Hwmons reads every /sys/class/hwmon/hwmon*, resolving each chip's of_node path
// (for profile matching) and its temp/fan/power channels.
func (r *Reader) Hwmons() []Hwmon {
	dirs, _ := filepath.Glob(filepath.Join(r.root(), "class/hwmon/hwmon*"))
	sort.Strings(dirs)
	out := make([]Hwmon, 0, len(dirs))
	for _, d := range dirs {
		h := Hwmon{
			Name:   readStr(filepath.Join(d, "name")),
			OfNode: ofNodePath(filepath.Join(d, "device", "of_node")),
			Dir:    d,
			Temp:   readChannels(d, "temp"),
			Fan:    readChannels(d, "fan"),
			Power:  readChannels64(d, "power"),
		}
		out = append(out, h)
	}
	return out
}

// ofNodePath resolves a device/of_node symlink to the device-tree node path it
// points at (e.g. ".../i2c-mux@70/i2c@0/power_sensor@41"). Empty when there is no
// device tree (x86) or the link is absent.
func ofNodePath(link string) string {
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return ""
	}
	// Keep the path from "base/" onward — that suffix is what a profile key
	// matches against; the mount prefix (…/firmware/devicetree/base) is noise.
	if i := strings.Index(target, "devicetree/base/"); i >= 0 {
		return target[i+len("devicetree/base/"):]
	}
	return target
}

// readChannels collects "<kind>N_input" readings keyed by channel index N.
func readChannels(dir, kind string) map[int]int {
	files, _ := filepath.Glob(filepath.Join(dir, kind+"*_input"))
	out := make(map[int]int, len(files))
	for _, f := range files {
		if n, ok := channelIndex(filepath.Base(f), kind); ok {
			out[n] = readInt(f)
		}
	}
	return out
}

func readChannels64(dir, kind string) map[int]int64 {
	files, _ := filepath.Glob(filepath.Join(dir, kind+"*_input"))
	out := make(map[int]int64, len(files))
	for _, f := range files {
		if n, ok := channelIndex(filepath.Base(f), kind); ok {
			out[n] = int64(readInt(f))
		}
	}
	return out
}

// channelIndex parses N from "<kind>N_input".
func channelIndex(base, kind string) (int, bool) {
	s := strings.TrimSuffix(strings.TrimPrefix(base, kind), "_input")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func readStr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readInt(path string) int {
	n, _ := strconv.Atoi(readStr(path))
	return n
}

// readIntOK reads an integer attribute, reporting present=false when the file is
// absent or unparsable — the caller needs to tell a real 0 from a missing limit.
func readIntOK(path string) (int, bool) {
	s := readStr(path)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readInt64OK is readIntOK for the 64-bit channels (power, optical microwatts).
func readInt64OK(path string) (int64, bool) {
	s := readStr(path)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readFlag reads a boolean sysfs attribute — a non-zero integer is true, and an
// absent file is false (the chip does not report that alarm).
func readFlag(path string) bool {
	n, ok := readIntOK(path)
	return ok && n != 0
}

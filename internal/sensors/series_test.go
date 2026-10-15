// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"maps"
	"testing"
)

// seriesByName maps each reading's display name to its series name, for the
// readings that have one name only.
func seriesByName(temps []TempReading) map[string]string {
	out := map[string]string{}
	for i, s := range SeriesNames(temps) {
		out[temps[i].Name+"@"+temps[i].Source] = s
	}
	return out
}

// TestASeriesIsNamedAsTheKernelNamesItsSensor: the DK's readings are named for
// the zone or the chip's channel, a shared channel name told apart by the
// chip's device-tree node, so a profile that renames a reading keeps its
// history.
func TestASeriesIsNamedAsTheKernelNamesItsSensor(t *testing.T) {
	const near, sfpArea = "soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c", "soc/i2c@2180000/i2c-mux@70/i2c@2/temp-sensor@4c"
	got := seriesByName(ResolveAll(dkProfile(), buildDK(t)).Temps)
	for reading, want := range map[string]string{
		"CPU@thermal zone":                   "cluster-thermal",
		"Memory@thermal zone":                "ddr-thermal",
		"Network engine@thermal zone":        "fman-thermal",
		"PCB near CPU@tmp431 · 5-4c":         "temp1." + near,
		"CPU (external diode)@tmp431 · 5-4c": "temp2." + near,
		"SFP+ modules@tmp431 · 6-4c":         "temp1." + sfpArea,
		"temperature@sfp · xfi0":             "temperature.sfp · xfi0",
		"temperature@sfp · xfi1":             "temperature.sfp · xfi1",
	} {
		if got[reading] != want {
			t.Errorf("%s: series %q, want %q", reading, got[reading], want)
		}
	}

	renamed := dkProfile()
	for i := range renamed.Thermal {
		renamed.Thermal[i].Name += " (renamed)"
	}
	again := SeriesNames(ResolveAll(renamed, buildDK(t)).Temps)
	before := SeriesNames(ResolveAll(dkProfile(), buildDK(t)).Temps)
	if !maps.Equal(setOf(again), setOf(before)) {
		t.Errorf("renaming the profile's readings moved their series:\n%q\n%q", before, again)
	}
}

// TestAnUnprofiledBoxKeepsItsSeries: a box no profile describes already
// recorded under the kernel's names, a shared one told apart by its chip, so
// its series are the names it has always had.
func TestAnUnprofiledBoxKeepsItsSeries(t *testing.T) {
	got := seriesByName(ResolveAll(nil, buildGeneric(t)).Temps)
	for reading, want := range map[string]string{
		"Tctl@k10temp":           "Tctl",
		"edge@amdgpu":            "edge",
		"Composite@nvme · 05:00": "Composite.nvme · 05:00",
		"Composite@nvme · 0e:00": "Composite.nvme · 0e:00",
	} {
		if got[reading] != want {
			t.Errorf("%s: series %q, want %q", reading, got[reading], want)
		}
	}
}

// buildTwinLM75 is an unprofiled board with a device tree whose two lm75
// chips both report temp1, and a zone.
func buildTwinLM75(t *testing.T) *Reader {
	t.Helper()
	root := t.TempDir()
	zoneOf(t, root, 0, "cpu-thermal", 51000, nil, "soc/thermal-sensor@0")
	a := chip(t, root, 1, "lm75", "5-0048", "soc/i2c@0/lm75@48")
	writeTemp(t, a, 1, 40000, 80000, 85000, false)
	b := chip(t, root, 2, "lm75", "5-0049", "soc/i2c@0/lm75@49")
	writeTemp(t, b, 1, 42000, 80000, 85000, false)
	return &Reader{Root: root + "/sys"}
}

// TestTheOldNamesMoveToTheKernels: a series each reading was recorded under
// before the kernel's names (its display name, told apart by its bus locator)
// is paired with the name it is recorded under now. A board's twin chips move
// from their bus addresses to their nodes; a name unchanged is no move.
func TestTheOldNamesMoveToTheKernels(t *testing.T) {
	got := SeriesRenames(ResolveAll(nil, buildTwinLM75(t)).Temps)
	want := map[string]string{
		"temp1.lm75 · 5-48": "temp1.soc/i2c@0/lm75@48",
		"temp1.lm75 · 5-49": "temp1.soc/i2c@0/lm75@49",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("renames %q, want %q", got, want)
	}

	// The DK's profile names move to the kernel's, every one but the name its
	// profile no longer gives (history step 9 moves that one).
	dk := SeriesRenames(ResolveAll(dkProfile(), buildDK(t)).Temps)
	if dk["Memory"] != "ddr-thermal" || dk["PCB near CPU"] != "temp1.soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c" {
		t.Errorf("DK renames %q", dk)
	}
	if _, ok := dk["temperature.sfp · xfi0"]; ok {
		t.Errorf("an unchanged name moves: %q", dk)
	}
}

// TestARenameNeverTakesAnotherSensorsSeries: an old name that another reading
// now records under is that reading's series, so it is not moved, however
// often the renames are asked.
func TestARenameNeverTakesAnotherSensorsSeries(t *testing.T) {
	temps := []TempReading{
		{Name: "cpu-thermal", Kernel: "soc-thermal", Kind: "zone"}, // a profile named this reading for another's kernel name
		{Name: "CPU", Kernel: "cpu-thermal", Kind: "zone"},
	}
	if got := SeriesRenames(temps); got["cpu-thermal"] != "" {
		t.Errorf("renames %q move cpu-thermal, which the second reading records under", got)
	}
}

func setOf(names []string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

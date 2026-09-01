// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"os"
	"path/filepath"
	"testing"
)

// The profile mirrors profiles/mono_gateway-dk/profile.json closely enough to
// exercise every resolution path: zone picks, the :tempN channel selector, the
// main power rail, and the main fan.
func dkProfile() *Profile {
	return &Profile{
		Name:  "Mono Gateway Development Kit",
		Ports: []string{"eth1", "eth2", "eth0", "eth3", "eth4"},
		Fans: []Entry{
			{Path: "i2c-mux@70/i2c@3/fan-controller@2e/fan@0", Name: "System Fan 1", Main: true},
			{Path: "i2c-mux@70/i2c@3/fan-controller@2e/fan@1", Name: "System Fan 2"},
		},
		Power: []Entry{
			{Path: "i2c-mux@70/i2c@0/power_sensor@40", Name: "USB Power Delivery", Main: true},
			{Path: "i2c-mux@70/i2c@0/power_sensor@41", Name: "5V PSU"},
			{Path: "i2c-mux@70/i2c@0/power_sensor@42", Name: "1V Core PSU"},
			{Path: "i2c-mux@70/i2c@1/power_sensor@43", Name: "3.3V PSU"},
		},
		Thermal: []Entry{
			{Path: "cluster-thermal", Name: "cluster", CPU: true},
			{Path: "ddr-thermal", Name: "Memory"},
			{Path: "fman-thermal", Name: "Network engine"},
			{Path: "serdes-thermal", Name: "SerDes"},
			{Path: "sec-thermal", Name: "Crypto engine"},
			{Path: "i2c-mux@70/i2c@1/temp-sensor@4c:temp1", Name: "PCB near CPU"},
			{Path: "i2c-mux@70/i2c@1/temp-sensor@4c:temp2", Name: "CPU (external diode)"},
			{Path: "i2c-mux@70/i2c@2/temp-sensor@4c:temp1", Name: "SFP+ modules"},
		},
	}
}

// buildDK writes a faithful Mono Gateway DK sysfs tree and returns a Reader.
func buildDK(t *testing.T) *Reader {
	t.Helper()
	root := t.TempDir()

	trips := [][2]string{{"passive", "85000"}, {"critical", "95000"}}
	zoneOf(t, root, 0, "cluster-thermal", 48000, trips, "soc/tmu@1f00000/cluster")
	zoneOf(t, root, 1, "ddr-thermal", 47000, trips, "soc/tmu@1f00000/ddr")
	zoneOf(t, root, 2, "fman-thermal", 48000, trips, "soc/tmu@1f00000/fman")
	zoneOf(t, root, 3, "serdes-thermal", 47000, trips, "soc/tmu@1f00000/serdes")
	zoneOf(t, root, 4, "sec-thermal", 47000, trips, "soc/tmu@1f00000/sec")

	// The TMU also exposes each site as a hwmon twin sharing the zone's of_node —
	// these must be deduped away, keeping the zone (which carries the trips).
	twin := chip(t, root, 5, "cluster_thermal", "tmu-hwmon0", "soc/tmu@1f00000/cluster")
	write(t, filepath.Join(twin, "temp1_input"), "48000")

	// Two tmp431 board sensors behind the i2c0 mux, each with a local and a remote
	// diode. The second's remote diode is unconnected (reads 0, fault set).
	near := chip(t, root, 6, "tmp431", "5-004c", "soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c")
	writeTemp(t, near, 1, 39300, 85000, 85000, false)
	writeTemp(t, near, 2, 48200, 85000, 85000, false)
	sfpArea := chip(t, root, 7, "tmp431", "6-004c", "soc/i2c@2180000/i2c-mux@70/i2c@2/temp-sensor@4c")
	writeTemp(t, sfpArea, 1, 46800, 85000, 85000, false)
	writeTemp(t, sfpArea, 2, 0, 85000, 85000, true) // unconnected remote → fault

	// emc2305 fan controller: one running fan, one empty header, both at 30% duty.
	fan := chip(t, root, 8, "emc2305", "7-002e", "soc/i2c@2180000/i2c-mux@70/i2c@3/fan-controller@2e")
	write(t, filepath.Join(fan, "fan1_input"), "2622")
	write(t, filepath.Join(fan, "fan2_input"), "0")
	write(t, filepath.Join(fan, "pwm1"), "76")
	write(t, filepath.Join(fan, "pwm2"), "76")

	// Eight ina234 rails; @40 on i2c@0 is the USB-C input (main). Watts are chosen
	// distinct so the sort is observable, not to match the illustrative mock.
	rails := []struct {
		hw      int
		bus, of string
		mv, ma  int
		uw      int64
	}{
		{10, "12-0040", "soc/i2c@21a0000/i2c-mux@70/i2c@0/power_sensor@40", 20100, 490, 9800000},
		{11, "12-0041", "soc/i2c@21a0000/i2c-mux@70/i2c@0/power_sensor@41", 4970, 420, 2100000},
		{12, "12-0042", "soc/i2c@21a0000/i2c-mux@70/i2c@0/power_sensor@42", 1000, 3100, 3100000},
		{13, "13-0043", "soc/i2c@21a0000/i2c-mux@70/i2c@1/power_sensor@43", 3300, 730, 2400000},
	}
	for _, rl := range rails {
		d := chip(t, root, rl.hw, "ina234", rl.bus, rl.of)
		write(t, filepath.Join(d, "power1_input"), itoa(int(rl.uw)))
		write(t, filepath.Join(d, "in1_input"), itoa(rl.mv))
		write(t, filepath.Join(d, "curr1_input"), itoa(rl.ma))
	}

	// Two SFP+ cages: ISA hwmon, no of_node, matched by name. xfi0 has light, xfi1
	// is below the receive floor (no signal).
	x0 := chip(t, root, 20, "sfp_xfi0", "", "")
	writeSFP(t, x0, sfpVals{temp: 47200, tmax: 73000, tcrit: 78000, vcc: 3300, vmin: 3000, vmax: 3600,
		bias: 9, bmax: 10, tx: 542, txmin: 200, txmax: 794, rx: 412, rxmin: 16, rxmax: 794})
	x1 := chip(t, root, 21, "sfp_xfi1", "", "")
	writeSFP(t, x1, sfpVals{temp: 54600, tmax: 90000, tcrit: 95000, vcc: 3260, vmin: 3100, vmax: 3500,
		bias: 20, bmax: 70, tx: 2150, txmin: 708, txmax: 5010, rx: 3, rxmin: 1, rxmax: 251})

	return &Reader{Root: filepath.Join(root, "sys")}
}

func TestResolveAllTemps(t *testing.T) {
	inv := ResolveAll(dkProfile(), buildDK(t))

	// The TMU twin is deduped away: five zones, four tmp431 channels, two SFP
	// temperatures — eleven readings, not the twelve a naive read would give.
	if got := len(inv.Temps); got != 11 {
		t.Fatalf("temps: got %d, want 11\n%s", got, dumpTemps(inv.Temps))
	}

	cpu := inv.CPUTemp()
	if cpu == nil || cpu.zoneType != "cluster-thermal" || cpu.MilliC != 48000 {
		t.Fatalf("cpu pick: %+v", cpu)
	}
	if cpu.Curated {
		t.Fatal("the CPU pick leads the grid and must not also sit in the curated list")
	}
	if cpu.Warn != 85000 || cpu.Crit != 95000 {
		t.Fatalf("cpu limits from zone trips: warn=%d crit=%d", cpu.Warn, cpu.Crit)
	}

	curated := inv.CuratedTemps()
	wantNames := []string{"Memory", "Network engine", "SerDes", "Crypto engine", "PCB near CPU", "CPU (external diode)", "SFP+ modules"}
	if len(curated) != len(wantNames) {
		t.Fatalf("curated: got %d, want %d\n%s", len(curated), len(wantNames), dumpTemps(curated))
	}
	for i, name := range wantNames {
		if curated[i].Name != name {
			t.Fatalf("curated[%d] = %q, want %q", i, curated[i].Name, name)
		}
	}

	// The unconnected remote diode is a fault: excluded from the curated list,
	// graded neutral, present in the full list to render "—".
	// The unconnected remote is the SFP-area chip's temp2, which the profile does
	// not curate; it stays a kernel-named, uncurated, neutral, present-in-table row.
	fault := findTemp(inv.Temps, func(x TempReading) bool { return x.Fault })
	if fault == nil || fault.MilliC != 0 || fault.Level != "" || fault.Curated || fault.Kernel != "temp2" {
		t.Fatalf("faulted diode: %+v", fault)
	}

	// Source locators read like sensors: chip · trimmed bus address.
	near := findTemp(inv.Temps, func(x TempReading) bool { return x.Name == "PCB near CPU" })
	if near == nil || near.Source != "tmp431 · 5-4c" {
		t.Fatalf("tmp431 source: %+v", near)
	}
	if near.Warn != 85000 || near.Crit != 85000 || near.Kernel != "temp1" {
		t.Fatalf("tmp431 near reading: %+v", near)
	}
}

func TestResolveAllPower(t *testing.T) {
	inv := ResolveAll(dkProfile(), buildDK(t))

	main := inv.MainPower()
	if main == nil || main.Name != "USB Power Delivery" || main.MicroW != 9800000 {
		t.Fatalf("main rail: %+v", main)
	}
	if !main.HasV || main.MilliV != 20100 || !main.HasA || main.MilliA != 490 {
		t.Fatalf("main rail V/A: %+v", main)
	}

	others := inv.OtherPowers()
	if len(others) != 3 {
		t.Fatalf("other rails: got %d, want 3", len(others))
	}
	// Sorted by draw, descending, and the main rail is not among them.
	if others[0].MicroW < others[1].MicroW || others[1].MicroW < others[2].MicroW {
		t.Fatalf("rails not sorted desc: %v", []int64{others[0].MicroW, others[1].MicroW, others[2].MicroW})
	}
	for _, p := range others {
		if p.Main {
			t.Fatal("main rail leaked into the itemization")
		}
	}
	if others[0].Source != "ina234 · 12-42" && others[0].Name != "1V Core PSU" {
		t.Logf("top rail: %+v", others[0])
	}
}

func TestResolveAllFans(t *testing.T) {
	inv := ResolveAll(dkProfile(), buildDK(t))

	if got := inv.RunningFans(); got != 1 {
		t.Fatalf("running fans: got %d, want 1", got)
	}
	main := inv.MainFan()
	if main == nil || main.Name != "System Fan 1" || main.RPM != 2622 {
		t.Fatalf("main fan: %+v", main)
	}
	if !main.HasDuty || main.Duty != 30 {
		t.Fatalf("fan duty: %+v", main)
	}
	empty := findFan(inv.Fans, func(f FanReading) bool { return f.Kernel == "fan2" })
	if empty == nil || empty.State != "absent" {
		t.Fatalf("empty header should be absent: %+v", empty)
	}
	// One emc2305 on the board, so it is unique and shows its bare driver name —
	// the bus address is disambiguation the board doesn't need here (unlike the
	// eight ina234 or two tmp431, which do earn theirs).
	if empty.Source != "emc2305" {
		t.Fatalf("fan source: %q", empty.Source)
	}
}

func TestResolveAllFibers(t *testing.T) {
	inv := ResolveAll(dkProfile(), buildDK(t))

	if len(inv.Fibers) != 2 {
		t.Fatalf("fibers: got %d, want 2", len(inv.Fibers))
	}
	var x0, x1 *FiberModule
	for i := range inv.Fibers {
		switch inv.Fibers[i].Cage {
		case "xfi0":
			x0 = &inv.Fibers[i]
		case "xfi1":
			x1 = &inv.Fibers[i]
		}
	}
	if x0 == nil || x1 == nil {
		t.Fatalf("cages: %+v", inv.Fibers)
	}
	if x0.Source != "sfp · xfi0" {
		t.Fatalf("fibre source: %q", x0.Source)
	}
	if !x0.RxHasLight {
		t.Fatal("xfi0 receives light and should read as linked")
	}
	if x1.RxHasLight {
		t.Fatal("xfi1 is below the receive floor and should read as no signal")
	}
	if x0.TxMicroW != 542 || x0.RxMicroW != 412 || x0.VccMilliV != 3300 || x0.BiasMilliA != 9 {
		t.Fatalf("xfi0 DOM: %+v", x0)
	}
	if x0.TempMilliC != 47200 || x0.TempCrit != 78000 {
		t.Fatalf("xfi0 temperature: %+v", x0)
	}
}

// --- unprofiled x86 box ----------------------------------------------------

func buildGeneric(t *testing.T) *Reader {
	t.Helper()
	root := t.TempDir()

	// k10temp: Tctl, no limits — the CPU pick, neutral, no bar. It sits on a real
	// PCI address, so the fixture proves a chip unique on the box shows its bare
	// name regardless of that address (only a shared name earns a locator).
	k10 := chip(t, root, 0, "k10temp", "0000:00:18.3", "")
	write(t, filepath.Join(k10, "temp1_input"), "77900")
	write(t, filepath.Join(k10, "temp1_label"), "Tctl")

	// amdgpu: edge (no limits) plus a PPT power channel and voltages the table lists.
	gpu := chip(t, root, 1, "amdgpu", "0000:0f:00.0", "")
	write(t, filepath.Join(gpu, "temp1_input"), "74000")
	write(t, filepath.Join(gpu, "temp1_label"), "edge")
	write(t, filepath.Join(gpu, "power1_input"), "7000")

	// Two NVMe drives: a Composite with real limits and two per-die sensors whose
	// limits are sentinels (+65261.8 °C / −273.1 °C) that must be discarded.
	for i, bus := range []string{"0000:05:00.0", "0000:0e:00.0"} {
		nv := chip(t, root, 2+i, "nvme", bus, "")
		writeTemp(t, nv, 1, 47900+int(i)*1000, 80800, 84800, false)
		write(t, filepath.Join(nv, "temp1_label"), "Composite")
		writeTemp(t, nv, 2, 47900+int(i)*1000, 65261800, 65261800, false)
		write(t, filepath.Join(nv, "temp2_label"), "Sensor 1")
		writeTemp(t, nv, 3, 50900+int(i)*1000, 65261800, 65261800, false)
		write(t, filepath.Join(nv, "temp3_label"), "Sensor 2")
	}
	return &Reader{Root: filepath.Join(root, "sys")}
}

func TestResolveAllGeneric(t *testing.T) {
	inv := ResolveAll(nil, buildGeneric(t))

	cpu := inv.CPUTemp()
	if cpu == nil || cpu.chip != "k10temp" || cpu.MilliC != 77900 {
		t.Fatalf("generic cpu pick: %+v", cpu)
	}
	if cpu.Level != "" || cpu.HasLimits {
		t.Fatalf("Tctl has no limits: %+v", cpu)
	}
	if cpu.Source != "k10temp" {
		t.Fatalf("a chip unique on the box shows its bare name even on PCI: %q", cpu.Source)
	}

	// Curated = each chip's primary channel except the CPU pick: edge, and one
	// Composite per drive. The per-die Sensor channels stay out of the section.
	curated := inv.CuratedTemps()
	names := map[string]int{}
	for _, c := range curated {
		names[c.Name]++
	}
	if names["edge"] != 1 || names["Composite"] != 2 {
		t.Fatalf("generic curated set wrong: %v\n%s", names, dumpTemps(curated))
	}
	if names["Sensor 1"] != 0 || names["Tctl"] != 0 {
		t.Fatalf("secondary/CPU channels leaked into curated: %v", names)
	}

	// The Composite carries its real NVMe limits; a per-die sensor's sentinels are
	// discarded (files seen, no usable limit).
	comp := findTemp(inv.Temps, func(x TempReading) bool { return x.Name == "Composite" })
	if comp == nil || !comp.HasLimits || comp.Crit != 84800 {
		t.Fatalf("composite limits: %+v", comp)
	}
	if comp.Source != "nvme · 05:00" {
		t.Fatalf("nvme source: %q", comp.Source)
	}
	sensor := findTemp(inv.Temps, func(x TempReading) bool { return x.Name == "Sensor 1" })
	if sensor == nil || sensor.HasLimits || !sensor.SentinelDiscarded {
		t.Fatalf("sentinel limits should be discarded: %+v", sensor)
	}

	// An unprofiled box names no main rail, so the page draws no Power section —
	// but the amdgpu PPT still lists in the full table.
	if inv.MainPower() != nil {
		t.Fatalf("unprofiled board must not flag a main rail: %+v", inv.MainPower())
	}
	if len(inv.Powers) != 1 || inv.Powers[0].MicroW != 7000 {
		t.Fatalf("amdgpu PPT should list: %+v", inv.Powers)
	}
	if len(inv.Fans) != 0 || len(inv.Fibers) != 0 {
		t.Fatalf("box reports no fans or fibre: fans=%d fibers=%d", len(inv.Fans), len(inv.Fibers))
	}
}

// TestGenericInstanceGrouping guards the real-hardware case the mock's PCI
// addresses hid: two identical drives whose hwmon device leaf carries no bus
// address collapse to the same display source ("nvme"), yet each must still be
// curated as its own chip — the primary grouping keys on the sysfs instance, not
// the source string.
func TestGenericInstanceGrouping(t *testing.T) {
	root := t.TempDir()
	for i, leaf := range []string{"nvme0", "nvme1"} {
		nv := chip(t, root, i, "nvme", leaf, "")
		writeTemp(t, nv, 1, 47000+i*1000, 80000, 84000, false)
		write(t, filepath.Join(nv, "temp1_label"), "Composite")
		writeTemp(t, nv, 2, 60000+i*1000, 65261800, 65261800, false)
		write(t, filepath.Join(nv, "temp2_label"), "Sensor 1")
	}
	inv := ResolveAll(nil, &Reader{Root: filepath.Join(root, "sys")})

	composites := 0
	for _, c := range inv.CuratedTemps() {
		if c.Name == "Composite" {
			composites++
		}
		if c.Source != "nvme" {
			t.Fatalf("a bus-less leaf should read as the bare driver name, got %q", c.Source)
		}
	}
	if composites != 2 {
		t.Fatalf("both drives must curate their own Composite, got %d\n%s", composites, dumpTemps(inv.CuratedTemps()))
	}
}

// --- fixture helpers -------------------------------------------------------

// chip writes a hwmonN with a name, a device symlink to a bus-address dir (so the
// source locator resolves), and an optional of_node under that device. Returns the
// hwmon dir. A blank bus omits the device symlink (an ISA chip like an SFP cage).
func chip(t *testing.T, root string, hwN int, name, bus, ofNode string) string {
	t.Helper()
	d := filepath.Join(root, "sys", "class", "hwmon", "hwmon"+itoa(hwN))
	write(t, filepath.Join(d, "name"), name)
	if bus == "" {
		return d
	}
	busDir := filepath.Join(root, "sys", "devices", bus)
	if err := os.MkdirAll(busDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(busDir, filepath.Join(d, "device")); err != nil {
		t.Fatal(err)
	}
	if ofNode != "" {
		target := filepath.Join(root, "firmware", "devicetree", "base", ofNode)
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(busDir, "of_node")); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// zoneOf writes a thermal zone with trips and an of_node (for twin dedup).
func zoneOf(t *testing.T, root string, n int, typ string, milliC int, trips [][2]string, ofNode string) {
	t.Helper()
	d := filepath.Join(root, "sys", "class", "thermal", "thermal_zone"+itoa(n))
	write(t, filepath.Join(d, "type"), typ)
	write(t, filepath.Join(d, "temp"), itoa(milliC))
	for i, tr := range trips {
		write(t, filepath.Join(d, "trip_point_"+itoa(i)+"_type"), tr[0])
		write(t, filepath.Join(d, "trip_point_"+itoa(i)+"_temp"), tr[1])
	}
	target := filepath.Join(root, "firmware", "devicetree", "base", ofNode)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "device"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(d, "device", "of_node")); err != nil {
		t.Fatal(err)
	}
}

func writeTemp(t *testing.T, dir string, ch, input, max, crit int, fault bool) {
	t.Helper()
	write(t, filepath.Join(dir, tempAttr(ch, "input")), itoa(input))
	write(t, filepath.Join(dir, tempAttr(ch, "max")), itoa(max))
	write(t, filepath.Join(dir, tempAttr(ch, "crit")), itoa(crit))
	if fault {
		write(t, filepath.Join(dir, tempAttr(ch, "fault")), "1")
	}
}

type sfpVals struct {
	temp, tmax, tcrit int
	vcc, vmin, vmax   int
	bias, bmax        int
	tx, txmin, txmax  int
	rx, rxmin, rxmax  int
}

func writeSFP(t *testing.T, dir string, v sfpVals) {
	t.Helper()
	write(t, filepath.Join(dir, "temp1_input"), itoa(v.temp))
	write(t, filepath.Join(dir, "temp1_label"), "temperature")
	write(t, filepath.Join(dir, "temp1_max"), itoa(v.tmax))
	write(t, filepath.Join(dir, "temp1_crit"), itoa(v.tcrit))
	write(t, filepath.Join(dir, "in0_input"), itoa(v.vcc))
	write(t, filepath.Join(dir, "in0_min"), itoa(v.vmin))
	write(t, filepath.Join(dir, "in0_max"), itoa(v.vmax))
	write(t, filepath.Join(dir, "curr1_input"), itoa(v.bias))
	write(t, filepath.Join(dir, "curr1_max"), itoa(v.bmax))
	write(t, filepath.Join(dir, "power1_input"), itoa(v.tx))
	write(t, filepath.Join(dir, "power1_min"), itoa(v.txmin))
	write(t, filepath.Join(dir, "power1_max"), itoa(v.txmax))
	write(t, filepath.Join(dir, "power2_input"), itoa(v.rx))
	write(t, filepath.Join(dir, "power2_min"), itoa(v.rxmin))
	write(t, filepath.Join(dir, "power2_max"), itoa(v.rxmax))
}

func findTemp(ts []TempReading, pred func(TempReading) bool) *TempReading {
	for i := range ts {
		if pred(ts[i]) {
			return &ts[i]
		}
	}
	return nil
}

func findFan(fs []FanReading, pred func(FanReading) bool) *FanReading {
	for i := range fs {
		if pred(fs[i]) {
			return &fs[i]
		}
	}
	return nil
}

func dumpTemps(ts []TempReading) string {
	var b []byte
	for _, x := range ts {
		b = append(b, []byte(x.Kind+" "+x.Name+" ("+x.Kernel+") "+x.Source+"\n")...)
	}
	return string(b)
}

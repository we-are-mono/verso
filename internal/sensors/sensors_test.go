// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// --- profile parsing -------------------------------------------------------

func TestLoadProfile(t *testing.T) {
	const js = `{
	  "name": "Mono Gateway Development Kit",
	  "ports": ["eth1", "eth0"],
	  "fans": [ { "i2c-mux@70/i2c@3/fan-controller@2e/fan@0": "System Fan 1" } ],
	  "power": [
	    { "i2c-mux@70/i2c@0/power_sensor@40": "USB Power Delivery" },
	    { "i2c-mux@70/i2c@0/power_sensor@41": "5V PSU", "main": true }
	  ],
	  "thermal": [ { "cluster-thermal": "cluster", "cpu": true } ]
	}`
	fsys := fstest.MapFS{"mono_gateway-dk/profile.json": {Data: []byte(js)}}

	p, ok := LoadProfile(fsys, "mono_gateway-dk")
	if !ok {
		t.Fatal("profile not loaded")
	}
	if p.Name != "Mono Gateway Development Kit" || len(p.Ports) != 2 {
		t.Fatalf("name/ports wrong: %q %v", p.Name, p.Ports)
	}
	main := p.Power[1]
	if main.Path != "i2c-mux@70/i2c@0/power_sensor@41" || main.Name != "5V PSU" || !main.Main {
		t.Fatalf("main power entry wrong: %+v", main)
	}
	if p.Power[0].Main {
		t.Fatal("non-main rail flagged main")
	}
	cpu := p.Thermal[0]
	if cpu.Path != "cluster-thermal" || !cpu.CPU {
		t.Fatalf("cpu thermal entry wrong: %+v", cpu)
	}
	if _, ok := LoadProfile(fsys, "no-such-board"); ok {
		t.Fatal("unknown board should not load")
	}
}

// --- resolution: profiled board (device tree) ------------------------------

func TestResolveProfiled(t *testing.T) {
	root := t.TempDir()
	// Thermal zones — the CPU site plus a second zone for the count.
	zone(t, root, 0, "cluster-thermal", 52000, [][2]string{{"passive", "85000"}, {"critical", "95000"}})
	zone(t, root, 1, "ddr-thermal", 48000, nil)
	// emc2305 fan controller: fan1 running, fan2 header empty.
	dt := hwmon(t, root, 0, "emc2305", "soc/i2c@2180000/i2c-mux@70/i2c@3/fan-controller@2e")
	write(t, filepath.Join(dt, "fan1_input"), "3630")
	write(t, filepath.Join(dt, "fan2_input"), "0")
	// Two INA power rails; @41 is the 5V main.
	main := hwmon(t, root, 1, "ina234", "soc/i2c@2180000/i2c-mux@70/i2c@0/power_sensor@41")
	write(t, filepath.Join(main, "power1_input"), "12400000") // 12.4 W
	aux := hwmon(t, root, 2, "ina234", "soc/i2c@2180000/i2c-mux@70/i2c@0/power_sensor@42")
	write(t, filepath.Join(aux, "power1_input"), "800000")

	p := &Profile{
		Fans:    []Entry{{Path: "i2c-mux@70/i2c@3/fan-controller@2e/fan@0", Name: "System Fan 1"}, {Path: "i2c-mux@70/i2c@3/fan-controller@2e/fan@1", Name: "System Fan 2"}},
		Power:   []Entry{{Path: "i2c-mux@70/i2c@0/power_sensor@41", Name: "5V PSU", Main: true}, {Path: "i2c-mux@70/i2c@0/power_sensor@42", Name: "1V"}},
		Thermal: []Entry{{Path: "cluster-thermal", Name: "cluster", CPU: true}},
	}
	f := Resolve(p, &Reader{Root: filepath.Join(root, "sys")})

	if f.CPUTemp == nil || f.CPUTemp.MilliC != 52000 {
		t.Fatalf("cpu temp: %+v", f.CPUTemp)
	}
	if f.CPUTemp.Status != "Normal" || f.CPUTemp.Level != "emerald" {
		t.Fatalf("cpu status: %+v", f.CPUTemp)
	}
	if f.Fan == nil || f.Fan.RPM != 3630 { // the running channel, not the empty header
		t.Fatalf("fan: %+v", f.Fan)
	}
	if f.Power == nil || f.Power.MicroW != 12400000 { // the main rail, not summed
		t.Fatalf("power: %+v", f.Power)
	}
	if f.PowerCount != 2 || f.ThermalCount != 2 {
		t.Fatalf("counts: power=%d thermal=%d", f.PowerCount, f.ThermalCount)
	}
}

// A hot reading past the passive trip grades Warm/amber.
func TestResolveWarm(t *testing.T) {
	root := t.TempDir()
	zone(t, root, 0, "cluster-thermal", 88000, [][2]string{{"passive", "85000"}, {"critical", "95000"}})
	p := &Profile{Thermal: []Entry{{Path: "cluster-thermal", CPU: true}}}
	f := Resolve(p, &Reader{Root: filepath.Join(root, "sys")})
	if f.CPUTemp == nil || f.CPUTemp.Status != "Warm" || f.CPUTemp.Level != "amber" {
		t.Fatalf("expected warm/amber: %+v", f.CPUTemp)
	}
}

// --- resolution: unprofiled board (x86, no device tree) --------------------

func TestResolveGeneric(t *testing.T) {
	root := t.TempDir()
	// coretemp exposes package + core temps; nct6775 a chassis fan. No of_node.
	ct := hwmon(t, root, 0, "coretemp", "")
	write(t, filepath.Join(ct, "temp1_input"), "45000")
	write(t, filepath.Join(ct, "temp2_input"), "40000")
	nct := hwmon(t, root, 1, "nct6775", "")
	write(t, filepath.Join(nct, "fan1_input"), "900")

	f := Resolve(nil, &Reader{Root: filepath.Join(root, "sys")})
	if f.CPUTemp == nil || f.CPUTemp.MilliC != 45000 {
		t.Fatalf("generic cpu temp: %+v", f.CPUTemp)
	}
	if f.Fan == nil || f.Fan.RPM != 900 {
		t.Fatalf("generic fan: %+v", f.Fan)
	}
	if f.Power != nil {
		t.Fatalf("generic power must hide, got %+v", f.Power)
	}
}

// --- fixture helpers -------------------------------------------------------

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// zone writes a thermal_zone with a type, reading, and trip points.
func zone(t *testing.T, root string, n int, typ string, milliC int, trips [][2]string) {
	t.Helper()
	d := filepath.Join(root, "sys", "class", "thermal", "thermal_zone"+itoa(n))
	write(t, filepath.Join(d, "type"), typ)
	write(t, filepath.Join(d, "temp"), itoa(milliC))
	for i, tr := range trips {
		write(t, filepath.Join(d, "trip_point_"+itoa(i)+"_type"), tr[0])
		write(t, filepath.Join(d, "trip_point_"+itoa(i)+"_temp"), tr[1])
	}
}

// hwmon writes a hwmonN with a name and (when ofNode is set) a device/of_node
// symlink into a fabricated device tree. Returns the hwmon dir.
func hwmon(t *testing.T, root string, n int, name, ofNode string) string {
	t.Helper()
	d := filepath.Join(root, "sys", "class", "hwmon", "hwmon"+itoa(n))
	write(t, filepath.Join(d, "name"), name)
	if ofNode != "" {
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
	return d
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

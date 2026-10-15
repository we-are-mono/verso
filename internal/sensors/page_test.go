// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"path/filepath"
	"slices"
	"testing"
)

// The Hardware page lists what a board's profile names, and nothing else: an
// entry left out of the profile's array stays off the page. A board with no
// profile lists everything its kernel exposes, under the kernel's names.

func tempNames(ts []TempReading) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}

// TestPageTempsListsTheProfilesEntries: on the DK, the CPU leads, the
// profile's entries follow in its order, and the unconnected remote diode and
// the SFP modules' own temperatures, which the profile does not name, are not
// listed.
func TestPageTempsListsTheProfilesEntries(t *testing.T) {
	got := tempNames(ResolveAll(dkProfile(), buildDK(t)).PageTemps())
	want := []string{"cluster", "Memory", "Network engine", "SerDes", "Crypto engine", "PCB near CPU", "CPU (external diode)", "SFP+ modules"}
	if !slices.Equal(got, want) {
		t.Fatalf("page temps = %q, want %q", got, want)
	}
}

// TestPageTempsUnprofiledListsEverything: with no profile, the CPU pick
// leads, each chip's primary channel follows in curation order, and the
// secondary channels close the list; an SFP module's temperature belongs to
// its fibre card.
func TestPageTempsUnprofiledListsEverything(t *testing.T) {
	r := buildGeneric(t)
	root := filepath.Dir(r.Root)
	sfp := chip(t, root, 9, "sfp_xfi0", "", "")
	writeSFP(t, sfp, sfpVals{temp: 47200, tmax: 73000, tcrit: 78000, vcc: 3300, vmin: 3000, vmax: 3600,
		bias: 9, bmax: 10, tx: 542, txmin: 200, txmax: 794, rx: 412, rxmin: 16, rxmax: 794})

	inv := ResolveAll(nil, r)
	got := tempNames(inv.PageTemps())
	want := []string{"Tctl", "edge", "Composite", "Composite", "Sensor 1", "Sensor 2", "Sensor 1", "Sensor 2"}
	if !slices.Equal(got, want) {
		t.Fatalf("page temps = %q, want %q\n%s", got, want, dumpTemps(inv.Temps))
	}
}

// TestPageTempsKeepsTheCPUOfAStaleProfile: a profile whose CPU entry matches
// nothing still leads with the CPU the kernel offers, as the Overview does.
func TestPageTempsKeepsTheCPUOfAStaleProfile(t *testing.T) {
	p := dkProfile()
	p.Thermal[0].Path = "no-such-thermal"
	got := ResolveAll(p, buildDK(t)).PageTemps()
	if len(got) == 0 || !got[0].CPU {
		t.Fatalf("a stale CPU entry lost the CPU: %q", tempNames(got))
	}
}

// TestPageRailsListsTheProfilesRails: the input leads, the rest follow by
// draw, and a rail the profile does not name is not listed.
func TestPageRailsListsTheProfilesRails(t *testing.T) {
	r := buildDK(t)
	extra := chip(t, filepath.Dir(r.Root), 14, "ina234", "13-0044", "soc/i2c@21a0000/i2c-mux@70/i2c@1/power_sensor@44")
	write(t, filepath.Join(extra, "power1_input"), "5000000")

	var got []string
	for _, p := range ResolveAll(dkProfile(), r).PageRails() {
		got = append(got, p.Name)
	}
	want := []string{"USB Power Delivery", "1V Core PSU", "3.3V PSU", "5V PSU"}
	if !slices.Equal(got, want) {
		t.Fatalf("page rails = %q, want %q", got, want)
	}
}

// TestPageRailsUnprofiledListsEveryChannelByDraw: with no profile there is no
// input to lead, so every channel lists by draw.
func TestPageRailsUnprofiledListsEveryChannelByDraw(t *testing.T) {
	r := buildGeneric(t)
	more := chip(t, filepath.Dir(r.Root), 9, "spd5118", "0-0051", "")
	write(t, filepath.Join(more, "power1_input"), "9000")

	rails := ResolveAll(nil, r).PageRails()
	if len(rails) != 2 || rails[0].MicroW != 9000 || rails[1].MicroW != 7000 {
		t.Fatalf("page rails = %+v, want both channels by draw", rails)
	}
	for _, p := range rails {
		if p.Main {
			t.Fatalf("an unprofiled rail is never the input: %+v", p)
		}
	}
}

// TestPageFansListsTheProfilesFans: the main fan leads, the rest follow in
// channel order, and a channel the profile does not name is not listed.
func TestPageFansListsTheProfilesFans(t *testing.T) {
	r := buildDK(t)
	write(t, filepath.Join(filepath.Dir(r.Root), "sys", "class", "hwmon", "hwmon8", "fan3_input"), "0")

	p := dkProfile()
	p.Fans[0].Main, p.Fans[1].Main = false, true
	var got []string
	for _, f := range ResolveAll(p, r).PageFans() {
		got = append(got, f.Name)
	}
	want := []string{"System Fan 2", "System Fan 1"}
	if !slices.Equal(got, want) {
		t.Fatalf("page fans = %q, want %q", got, want)
	}
}

// TestPageFansUnprofiledListsEveryChannel: with no profile every fan channel
// lists, under the kernel's name, in channel order.
func TestPageFansUnprofiledListsEveryChannel(t *testing.T) {
	r := buildGeneric(t)
	fan := chip(t, filepath.Dir(r.Root), 9, "nct6798", "0-0290", "")
	write(t, filepath.Join(fan, "fan1_input"), "1100")
	write(t, filepath.Join(fan, "fan2_input"), "0")

	var got []string
	for _, f := range ResolveAll(nil, r).PageFans() {
		got = append(got, f.Name)
	}
	if want := []string{"fan1", "fan2"}; !slices.Equal(got, want) {
		t.Fatalf("page fans = %q, want %q", got, want)
	}
}

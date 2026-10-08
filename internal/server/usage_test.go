// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
)

// usageServer is the roster with usage on: nlbwmon's days for the iPhone, and
// conntrack's running bytes for its addresses, which the test moves on.
func usageServer(t *testing.T, received *uint64) (*Server, *time.Time) {
	t.Helper()
	b := rosterBackend()
	b.usageDays = &openwrt.UsageDays{Enabled: true, Today: "2026-10-07", Days: map[string]map[string]openwrt.UsageCounter{
		"2026-10-07": {"42:e6:ad:ff:b7:af": {Received: 2 << 30, Sent: 1 << 29}},
		"2026-10-01": {"42:e6:ad:ff:b7:af": {Received: 1 << 30}, "a2:8c:d7:4a:0e:57": {Received: 512 << 20}},
		"2026-09-20": {"42:e6:ad:ff:b7:af": {Received: 9 << 30}},
	}}
	b.usageLive = func(addr string) openwrt.UsageCounter {
		if addr == "192.168.77.102" {
			return openwrt.UsageCounter{Received: *received, Sent: *received / 10}
		}
		return openwrt.UsageCounter{}
	}
	s := newServerWith(t, b, &fakeTransport{}, []plugin.Manifest{dnsdhcpManifest()})
	s.readLeases = rosterServer(t).readLeases
	s.neighbors = testNeighbors
	clock := time.Unix(1_000_000, 0)
	s.usageSampler.Now = func() time.Time { return clock }
	return s, &clock
}

// TestDevicesRosterSaysWhatEachDeviceMoves: with usage on, the roster's MAC
// column gives way to Now and This month (ADR-018). This month is the calendar
// month's days summed; Now is two readings apart, so the first page has none to
// state and the next, two seconds on, does.
func TestDevicesRosterSaysWhatEachDeviceMoves(t *testing.T) {
	var received uint64 = 1_000_000
	s, clock := usageServer(t, &received)

	body := get(t, s, "/devices").Body.String()
	for _, want := range []string{">Now · Mbit/s<", ">This month<", ">3.5 GiB<", ">512 MiB<"} {
		if !strings.Contains(body, want) {
			t.Errorf("usage roster missing %q", want)
		}
	}
	if strings.Contains(body, ">MAC address<") {
		t.Error("with usage on, the MAC gives its column to usage")
	}
	if strings.Contains(body, "data-verso-figure-text>8<") {
		t.Error("one reading is no rate")
	}

	*clock = clock.Add(2 * time.Second)
	received += 2_000_000 // 16 Mbit over two seconds
	body = get(t, s, "/devices").Body.String()
	if !strings.Contains(body, "data-verso-figure-text>8</span>") {
		t.Errorf("the second reading states the rate: 8 Mbit/s down\n%s", body[strings.Index(body, "usage-now"):][:600])
	}
	// 8.8 Mbit/s both ways is an HD film or a call: light, inked green.
	if !strings.Contains(body, `data-verso-ink="success"`) {
		t.Error("a device moving 8.8 Mbit/s reads as light")
	}
}

// TestDevicePanelDetailsSayWhatTheDeviceMoves: with usage on, the device's
// Details tab goes on, under its machine facts, to a Usage section: its live
// traffic graph, as the overview draws the Internet's, named so the roster's
// stream feeds it, and its periods by the calendar — today, the last seven
// days, the month so far and the month before — each down and up. Usage is not
// a tab of its own.
func TestDevicePanelDetailsSayWhatTheDeviceMoves(t *testing.T) {
	var received uint64 = 1_000_000
	s, _ := usageServer(t, &received)
	body := get(t, s, "/entity/device/42:e6:ad:ff:b7:af").Body.String()
	facts, section := strings.Index(body, ">MAC address<"), strings.Index(body, ">Usage<")
	if facts < 0 || section < facts {
		t.Fatalf("the Usage section follows the machine facts in Details:\n%s", body)
	}
	if strings.Contains(body, "?tab=usage") {
		t.Error("usage is a section of Details, not a tab")
	}
	for _, want := range []string{
		`data-verso-live-chart="usage:42:e6:ad:ff:b7:af"`, "Mbit/s down", "Mbit/s up", `<svg class="verso-chart`,
		">Downloaded<", ">Uploaded<",
		">Today<", ">Last 7 days<", ">This month<", ">Last month<",
		">2 GiB<", ">512 MiB<", // today down and up
		">3 GiB<", // this month's download: today and the 1st
		">9 GiB<", // last month's download
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Usage tab missing %q", want)
		}
	}

	if off := get(t, rosterServer(t), "/entity/device/42:e6:ad:ff:b7:af").Body.String(); strings.Contains(off, ">Usage<") {
		t.Error("with usage off Details has no Usage section")
	}
}

// TestAReadWithoutTodayKeepsTheDaysHeld: a history read that cannot say what
// today is has read nothing, so the month the shell already holds stands
// rather than going blank.
func TestAReadWithoutTodayKeepsTheDaysHeld(t *testing.T) {
	var received uint64
	s, _ := usageServer(t, &received)
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5 GiB<") {
		t.Fatal("the first read should give the month")
	}
	days := s.backend.(fakeBackend).usageDays
	days.Today, days.Days = "", nil
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5 GiB<") {
		t.Error("a read with no today must not blank the month already held")
	}
}

// TestUsageSpeaksSlovenian: the i18n audit's fake router has usage off, so the
// usage roster and panel would be its blind spot; this renders both in
// Slovenian from the real catalogs, every word of theirs translated.
func TestUsageSpeaksSlovenian(t *testing.T) {
	var received uint64
	s, _ := usageServer(t, &received)
	bundle, problems := i18n.Load(os.DirFS("../../i18n"), "*/*.json")
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	s.SetBundle(bundle)
	roster := getLang(t, s, "/devices", "sl")
	for _, want := range []string{">Zdaj · Mbit/s<", ">Ta mesec<", ">prenos<", ">nalaganje<",
		">majhna<", ">pod 10 Mbit/s<", ">srednja<", ">10 do 100 Mbit/s<", ">velika<", ">100 Mbit/s in več<"} {
		if !strings.Contains(roster, want) {
			t.Errorf("Slovenian roster missing %q", want)
		}
	}
	panel := getLang(t, s, "/entity/device/42:e6:ad:ff:b7:af", "sl")
	for _, want := range []string{">Naslov MAC<", ">Zakup<", ">Poraba</h2>", "Promet naprave — prenos in nalaganje, zadnja minuta",
		">Obdobje<", ">Preneseno<", ">Naloženo<", ">Danes<", ">Zadnjih 7 dni<", ">Ta mesec<", ">Prejšnji mesec<"} {
		if !strings.Contains(panel, want) {
			t.Errorf("Slovenian panel missing %q", want)
		}
	}
}

// TestDevicesRosterWithoutUsage: usage off (nlbwmon disabled) is the roster as
// it was, MAC and all, with no usage drawn.
func TestDevicesRosterWithoutUsage(t *testing.T) {
	body := get(t, rosterServer(t), "/devices").Body.String()
	if strings.Contains(body, "usage-now") || !strings.Contains(body, ">MAC address<") {
		t.Error("with usage off, the roster keeps its MAC column and draws no usage")
	}
}

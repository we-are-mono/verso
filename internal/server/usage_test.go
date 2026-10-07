// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"
	"time"

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
}

// TestDevicePanelDetailsSayWhatTheDeviceMoves: with usage on, the device's
// Details tab goes on, under its machine facts, to a Usage section: its rate
// now, down and up, named so the roster's stream keeps it current, and its
// periods by the calendar — today, the last seven days, the month so far and
// the month before — each down and up. Usage is not a tab of its own.
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
		`data-verso-stat="usage-down:42:e6:ad:ff:b7:af"`, `data-verso-stat="usage-up:42:e6:ad:ff:b7:af"`,
		">Download<", ">Upload<", "Mbit/s",
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

// TestDevicesRosterWithoutUsage: usage off (nlbwmon disabled) is the roster as
// it was, MAC and all, with no usage drawn.
func TestDevicesRosterWithoutUsage(t *testing.T) {
	body := get(t, rosterServer(t), "/devices").Body.String()
	if strings.Contains(body, "usage-now") || !strings.Contains(body, ">MAC address<") {
		t.Error("with usage off, the roster keeps its MAC column and draws no usage")
	}
}

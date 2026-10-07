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

// TestDevicesRosterWithoutUsage: usage off (nlbwmon disabled) is the roster as
// it was, MAC and all, with no usage drawn.
func TestDevicesRosterWithoutUsage(t *testing.T) {
	body := get(t, rosterServer(t), "/devices").Body.String()
	if strings.Contains(body, "usage-now") || !strings.Contains(body, ">MAC address<") {
		t.Error("with usage off, the roster keeps its MAC column and draws no usage")
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	for _, want := range []string{">Now<", ">This month<", ">3.5\U0000202fGiB<", ">512\U0000202fMiB<"} {
		if !strings.Contains(body, want) {
			t.Errorf("usage roster missing %q", want)
		}
	}
	if strings.Contains(body, ">MAC address<") {
		t.Error("with usage on, the MAC gives its column to usage")
	}
	if strings.Contains(body, "data-verso-figure-text>8\U0000202fMb/s<") {
		t.Error("one reading is no rate")
	}

	*clock = clock.Add(2 * time.Second)
	received += 2_000_000 // 16 Mbit over two seconds
	body = get(t, s, "/devices").Body.String()
	if !strings.Contains(body, "data-verso-figure-text>8\U0000202fMb/s</span>") {
		t.Errorf("the second reading states the rate: 8 Mb/s down\n%s", body[strings.Index(body, "usage-now"):][:600])
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
		`data-verso-live-chart="usage:42:e6:ad:ff:b7:af"`, `data-verso-live-lane="down"`, `data-verso-live-lane="up"`,
		">Download<", ">Upload<", `data-verso-place="compact"`, `<svg class="verso-chart__svg"`,
		">Downloaded<", ">Uploaded<",
		">Today<", ">Last 7 days<", ">This month<", ">Last month<",
		">2\U0000202fGiB<", ">512\U0000202fMiB<", // today down and up
		">3\U0000202fGiB<", // this month's download: today and the 1st
		">9\U0000202fGiB<", // last month's download
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
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Fatal("the first read should give the month")
	}
	days := s.backend.(fakeBackend).usageDays
	days.Today, days.Days = "", nil
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Error("a read with no today must not blank the month already held")
	}
}

// TestUsageIsShownOnlyUnderTheReadersGrant: the shell holds one copy of what
// the helper last answered, shared by every page, so a session rpcd refuses
// usageDays and usageLive sees none of it — not on the roster, not in a
// device's panel, not on the usage stream — whoever filled it a moment ago.
// Root's '*' sees usage as before, each grant asked once a page.
func TestUsageIsShownOnlyUnderTheReadersGrant(t *testing.T) {
	var received uint64 = 1_000_000
	s, clock := usageServer(t, &received)
	root := s.backend.(fakeBackend)
	var probes []string
	root.probes = &probes
	s.backend = root
	get(t, s, "/devices")
	*clock = clock.Add(2 * time.Second)
	received += 2_000_000
	body := get(t, s, "/devices").Body.String()
	for _, want := range []string{">This month<", ">3.5\U0000202fGiB<", ">512\U0000202fMiB<", "data-verso-figure-text>8\U0000202fMb/s<"} {
		if !strings.Contains(body, want) {
			t.Fatalf("root's roster missing %q", want)
		}
	}
	for _, grant := range []string{"ubus verso usageDays", "ubus verso usageLive"} {
		if n := strings.Count(strings.Join(probes, "\n"), grant); n != 2 {
			t.Errorf("%s probed %d times over two pages, want once each: %q", grant, n, probes)
		}
	}

	refused := root
	refused.probes = nil
	refused.grant = grantAllBut("ubus verso usageDays", "ubus verso usageLive")
	refused.usageErr = openwrt.ErrAccessDenied
	s.backend = refused
	*clock = clock.Add(100 * time.Millisecond) // inside the sampler's gap
	body = get(t, s, "/devices").Body.String()
	for _, leaked := range []string{">This month<", ">3.5\U0000202fGiB<", ">512\U0000202fMiB<", "8\U0000202fMb/s"} {
		if strings.Contains(body, leaked) {
			t.Errorf("a refused session's roster shows %q", leaked)
		}
	}
	panel := get(t, s, "/entity/device/42:e6:ad:ff:b7:af").Body.String()
	for _, leaked := range []string{">Usage<", ">2\U0000202fGiB<", ">3\U0000202fGiB<", ">9\U0000202fGiB<", "data-verso-live-chart"} {
		if strings.Contains(panel, leaked) {
			t.Errorf("a refused session's device panel shows %q", leaked)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // one frame, then the stream ends
	stream := serve(t, s, httptest.NewRequestWithContext(ctx, http.MethodGet, "/devices/usage", nil)).Body.String()
	if !strings.Contains(stream, `"devices":[]`) || strings.Contains(stream, "GiB") {
		t.Errorf("a refused session's usage stream = %s", stream)
	}

	// The days without the live read: the periods, and no graph of the
	// minute another session watched.
	daysOnly := root
	daysOnly.probes = nil
	daysOnly.grant = grantAllBut("ubus verso usageLive")
	s.backend = daysOnly
	panel = get(t, s, "/entity/device/42:e6:ad:ff:b7:af").Body.String()
	if !strings.Contains(panel, ">This month<") || strings.Contains(panel, "data-verso-live-chart") {
		t.Errorf("a session granted only the days: periods = %v, graph = %v",
			strings.Contains(panel, ">This month<"), strings.Contains(panel, "data-verso-live-chart"))
	}
	if body = get(t, s, "/devices").Body.String(); strings.Contains(body, "8\U0000202fMb/s") {
		t.Error("a session refused the live read is shown no rate")
	}
}

// TestUsageUncheckedSaysSo: an rpcd that answers the lease grant but not
// usageDays withholds usage as a refusal does, and the roster says the
// permissions could not be checked over its rows, and a device's panel under
// its Usage heading, rather than drawing the roster as if usage were off.
// Granted, neither says so.
func TestUsageUncheckedSaysSo(t *testing.T) {
	var received uint64 = 1_000_000
	s, _ := usageServer(t, &received)
	for _, path := range []string{"/devices", "/entity/device/42:e6:ad:ff:b7:af"} {
		if body := get(t, s, path).Body.String(); strings.Contains(body, permissionsUnchecked) {
			t.Errorf("%s says the permissions could not be checked with every grant answered", path)
		}
	}
	unchecked := s.backend.(fakeBackend)
	unchecked.unanswered = []string{"ubus verso usageDays"}
	s.backend = unchecked

	body := get(t, s, "/devices").Body.String()
	if strings.Contains(body, ">This month<") || strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Error("an unchecked grant still shows usage")
	}
	if !strings.Contains(body, permissionsUnchecked) || !strings.Contains(body, "toms-iphone") {
		t.Error("the roster drops usage without saying the permissions could not be checked")
	}
	panel := get(t, s, "/entity/device/42:e6:ad:ff:b7:af").Body.String()
	if strings.Contains(panel, ">2\U0000202fGiB<") || strings.Contains(panel, "data-verso-live-chart") {
		t.Error("an unchecked grant still shows the device's usage")
	}
	usage := strings.Index(panel, ">Usage<")
	if usage < 0 || !strings.Contains(panel[usage:], permissionsUnchecked) {
		t.Error("the device's panel drops usage without saying the permissions could not be checked")
	}
}

// TestUsageHeldStandsOnlyForAGrantedReader: a granted session whose read
// fails on the way keeps the month already held; one the helper itself
// refuses is refused what is held too.
func TestUsageHeldStandsOnlyForAGrantedReader(t *testing.T) {
	var received uint64
	s, _ := usageServer(t, &received)
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Fatal("the first read should give the month")
	}
	failing := s.backend.(fakeBackend)
	failing.usageErr = errors.New("helper unreachable")
	s.backend = failing
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Error("a failed read must not blank the month already held")
	}
	failing.usageErr = openwrt.ErrAccessDenied
	s.backend = failing
	if body := get(t, s, "/devices").Body.String(); strings.Contains(body, ">3.5\U0000202fGiB<") {
		t.Error("a read the helper refuses must not fall back to the month held")
	}
}

// TestUsageStreamStaysOffBetweenItsMonths: a session refused the days has
// usage off (ADR-018), on the stream as on the page: the ticks between two
// reads of the month carry no rate, though rpcd grants the session the live
// read.
func TestUsageStreamStaysOffBetweenItsMonths(t *testing.T) {
	var received uint64 = 1_000_000
	s, clock := usageServer(t, &received)
	get(t, s, "/devices")
	*clock = clock.Add(2 * time.Second)
	received += 2_000_000
	if body := get(t, s, "/devices").Body.String(); !strings.Contains(body, "data-verso-figure-text>8\U0000202fMb/s<") {
		t.Fatal("root's second page should state a rate")
	}
	liveOnly := s.backend.(fakeBackend)
	liveOnly.grant = grantAllBut("ubus verso usageDays")
	s.backend = liveOnly
	s.eventInterval = 5 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	stream := serve(t, s, httptest.NewRequestWithContext(ctx, http.MethodGet, "/devices/usage", nil)).Body.String()
	if n := strings.Count(stream, "event: usage"); n < 3 {
		t.Fatalf("the stream sent %d frames, want several: %s", n, stream)
	}
	if strings.Contains(stream, `"mac"`) {
		t.Errorf("a session refused the days is streamed rates: %s", stream)
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
	for _, want := range []string{">Zdaj<", ">Ta mesec<", ">prenos<", ">nalaganje<", ">3,5\U0000202fGiB<"} {
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/telemetry"
)

// getSignedIn fetches path as a signed-in operator.
func getSignedIn(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

// TestOverviewTrafficDrawsAPastWindowFromHistory: the Internet graph's window
// fetches the uplink's history, in Mbit/s, each minute at its own time with
// gaps where nothing was recorded, its figures the window's averages; the
// minute is the live graph again; a window not offered is refused.
func TestOverviewTrafficDrawsAPastWindowFromHistory(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, wan: openwrt.WANState{Devices: []openwrt.WANDevice{
		{Device: "wan0", Routes: []openwrt.WANRoute{{Family: 4, Main: true}}},
	}}})
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	now := time.Now().Truncate(time.Minute)
	store.Add(
		history.Point{Series: "iface.wan0.rx", Time: now.Add(-10 * time.Minute), Average: 20e6, Peak: 30e6},
		history.Point{Series: "iface.wan0.rx", Time: now.Add(-9 * time.Minute), Average: 40e6, Peak: 50e6},
		history.Point{Series: "iface.wan0.tx", Time: now.Add(-10 * time.Minute), Average: 1e6, Peak: 1e6},
		history.Point{Series: "iface.wan0.tx", Time: now.Add(-9 * time.Minute), Average: 3e6, Peak: 3e6},
		history.Point{Series: "wan.gateway.latency", Time: now.Add(-10 * time.Minute), Average: 10, Peak: 14},
		history.Point{Series: "wan.gateway.latency", Time: now.Add(-9 * time.Minute), Average: 14, Peak: 30},
		history.Point{Series: "wan.gateway.loss", Time: now.Add(-10 * time.Minute), Average: 0, Peak: 0},
		history.Point{Series: "wan.gateway.loss", Time: now.Add(-9 * time.Minute), Average: 33.3, Peak: 100},
	)

	for _, e := range []history.Event{
		{Time: now.Add(-30 * time.Minute), Kind: "wan.down", Subject: "wan0"},
		{Time: now.Add(-20 * time.Minute), Kind: "wan.up", Subject: "wan0", Detail: `{"after":600}`},
	} {
		if err := store.Record(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}

	status, body := getSignedIn(t, s, "/overview/traffic?range=day")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", status, body)
	}
	for _, want := range []string{
		">24\U0000202fh<", ">18\U0000202fh<", ">30\U0000202fMb/s<", ">2\U0000202fMb/s<", ">on average<",
		// Each lane's peak is the highest the history kept, not the averages'.
		">50\U0000202fMb/s<", ">3\U0000202fMb/s<",
		`class="verso-chart__line"`, `class="verso-chart__mark" data-verso-kind="outage"`, "<title>Internet down ",
		`class="verso-chart__mark" data-verso-kind="loss"`, "<title>Packet loss up to 33\U0000202f% ", ">Gateway<", ">12&#x202F;ms<",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a day's graph missing %q", want)
		}
	}
	if strings.Contains(body, "data-verso-live-chart") {
		t.Error("a day's graph is fed by the live stream")
	}
	// The window lives in the card's header: what comes back is the graph
	// alone, in its two lanes, under the id the header's dropdown targets.
	if !strings.Contains(body, `id="overview-traffic-graph"`) || !strings.Contains(body, `data-verso-live-lane="up"`) {
		t.Error("the fetched graph is not the addressed two-lane graph")
	}
	if strings.Contains(body, "<select") {
		t.Error("the fetched graph carries a window of its own; the card's header holds it")
	}

	if _, live := getSignedIn(t, s, "/overview/traffic?range=minute"); !strings.Contains(live, `data-verso-live-chart="wan"`) {
		t.Error("the minute is not the live graph")
	}
	if status, _ := getSignedIn(t, s, "/overview/traffic?range=decade"); status != http.StatusBadRequest {
		t.Errorf("an unknown window answered %d, want 400", status)
	}
}

// TestOverviewPastTrafficNeedsTheInterfaceGrant: history.db's uplink traffic,
// gateway loss and latency and its outages are the shell's own store, so a
// session rpcd refuses the interface dump, or does not answer for, is offered
// no past window and is refused one, while the live minute stays; the router
// starting is marked only under the system grant, as on the events page.
func TestOverviewPastTrafficNeedsTheInterfaceGrant(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	server := func(b fakeBackend) *Server {
		b.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "wan0", Routes: []openwrt.WANRoute{{Family: 4, Main: true}}}}}
		s := newServer(t, b)
		store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		s.historyStore = store
		store.Add(history.Point{Series: "iface.wan0.rx", Time: now.Add(-10 * time.Minute), Average: 20e6, Peak: 30e6})
		for _, e := range []history.Event{
			{Time: now.Add(-40 * time.Minute), Kind: "boot"},
			{Time: now.Add(-30 * time.Minute), Kind: "wan.down", Subject: "wan0"},
			{Time: now.Add(-20 * time.Minute), Kind: "wan.up", Subject: "wan0", Detail: `{"after":600}`},
		} {
			if err := store.Record(t.Context(), e); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	for _, c := range []struct {
		name, says string
		b          fakeBackend
	}{
		{"refused", trafficHistoryRefused, fakeBackend{grant: grantAllBut("ubus network.interface dump")}},
		{"unanswered", permissionsUnchecked, fakeBackend{accessErr: errors.New("rpcd gone")}},
	} {
		s := server(c.b)
		_, body := getSignedIn(t, s, "/")
		if strings.Contains(body, `name="range"`) {
			t.Errorf("%s: the overview offers a past window", c.name)
		}
		// The Traffic card says why it has no past windows, as the roster
		// says why it has no devices (ADR-007 §9).
		if card := body[strings.Index(body, `data-overview-card="traffic"`):]; !strings.Contains(card[:strings.Index(card, "</article>")], c.says) {
			t.Errorf("%s: the Traffic card does not say %q", c.name, c.says)
		}
		if status, body := getSignedIn(t, s, "/overview/traffic?range=day"); status != http.StatusForbidden || strings.Contains(body, "verso-chart__mark") {
			t.Errorf("%s: a past window answered %d:\n%s", c.name, status, body)
		}
		if _, live := getSignedIn(t, s, "/overview/traffic?range=minute"); !strings.Contains(live, `data-verso-live-chart="wan"`) {
			t.Errorf("%s: the live minute is withheld", c.name)
		}
	}

	s := server(fakeBackend{grant: grantAllBut("ubus system info")})
	_, body := getSignedIn(t, s, "/overview/traffic?range=day")
	if !strings.Contains(body, `data-verso-kind="outage"`) || strings.Contains(body, `data-verso-kind="boot"`) {
		t.Errorf("refused the system grant, the graph's marks:\n%s", body)
	}
	s = server(fakeBackend{access: true})
	if _, body := getSignedIn(t, s, "/"); !strings.Contains(body, `name="range"`) || strings.Contains(body, trafficHistoryRefused) {
		t.Error("root's overview offers no past window, or says it may not see one")
	}
	if _, body := getSignedIn(t, s, "/overview/traffic?range=day"); !strings.Contains(body, `data-verso-kind="boot"`) {
		t.Error("root's graph does not mark the router starting")
	}
}

// TestOverviewOffersTheWindowOnlyWithHistory: without a history to read, the
// Internet graph is the live minute alone, with no dropdown.
func TestOverviewOffersTheWindowOnlyWithHistory(t *testing.T) {
	s := newServer(t, fakeBackend{})
	if _, body := getSignedIn(t, s, "/"); strings.Contains(body, `name="range"`) {
		t.Error("the overview offers a window with no history behind it")
	}
	if status, _ := getSignedIn(t, s, "/overview/traffic?range=day"); status != http.StatusNotFound {
		t.Errorf("a window with no history answered %d, want 404", status)
	}
}

// TestInterfaceRatesHandsOverEachSecondOnce: each per-second rate reaches the
// history once, however the ticks fall. Loopback is not recorded, nor a device
// that is down: an idle tunnel placeholder or a port without link is a gap,
// not a line of zeros.
func TestInterfaceRatesHandsOverEachSecondOnce(t *testing.T) {
	at := func(s int64) uint64 { return uint64(s * 1000) }
	snapshot := telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth0", History: []telemetry.Point{
			{TimestampMS: at(1), RxBPS: 10, TxBPS: 1},
			{TimestampMS: at(2), RxBPS: 20, TxBPS: 2},
		}},
		{Name: "lo", History: []telemetry.Point{{TimestampMS: at(2), RxBPS: 5}}},
		{Name: "sit0", Operstate: "down", History: []telemetry.Point{{TimestampMS: at(2)}}},
	}}
	fake := &fakeTelemetry{snapshot: snapshot}
	source := interfaceRates(fake)

	got := source(time.Unix(2, 0))
	if !slices.Equal(got["iface.eth0.rx"], []float64{10, 20}) || !slices.Equal(got["iface.eth0.tx"], []float64{1, 2}) {
		t.Fatalf("first reading = %v, want both seconds of eth0", got)
	}
	if _, ok := got["iface.lo.rx"]; ok {
		t.Fatal("loopback was recorded")
	}
	if _, ok := got["iface.sit0.rx"]; ok {
		t.Fatal("a device that is down was recorded")
	}

	fake.snapshot.Interfaces[0].History = []telemetry.Point{
		{TimestampMS: at(2), RxBPS: 20, TxBPS: 2},
		{TimestampMS: at(3), RxBPS: 30, TxBPS: 3},
	}
	got = source(time.Unix(3, 0))
	if !slices.Equal(got["iface.eth0.rx"], []float64{30}) {
		t.Fatalf("second reading = %v, want only the new second", got)
	}
}

// TestMemoryInUseIsTotalLessAvailable: memory the kernel could hand out at
// once (page cache included) is not in use.
func TestMemoryInUseIsTotalLessAvailable(t *testing.T) {
	meminfo := []byte("MemTotal:        1000 kB\nMemFree:          100 kB\nMemAvailable:     600 kB\n")
	got, err := memoryInUse(meminfo)
	if err != nil || got != 400*1024 {
		t.Fatalf("memoryInUse = %v, %v; want 409600 bytes", got, err)
	}
	if _, err := memoryInUse([]byte("MemTotal: 1000 kB\n")); err == nil {
		t.Fatal("a meminfo without MemAvailable read as a value")
	}
}

// TestTemperatureSeriesRecordEachSensorUnderItsSeries: each working reading is
// recorded under the series it resolved with (the kernel's name, told apart by
// chip where shared: sensors.SeriesNames), never as a profile names it; a
// faulted reading is a gap.
func TestTemperatureSeriesRecordEachSensorUnderItsSeries(t *testing.T) {
	got := temperatureSeries([]sensors.TempReading{
		{Name: "Composite", Kernel: "Composite", Source: "nvme · 0f:00", MilliC: 46000, Series: "temp.Composite.nvme · 0f:00"},
		{Name: "CPU", Kernel: "cluster-thermal", Source: "thermal zone", Kind: "zone", MilliC: 80500, Series: "temp.cluster-thermal"},
		{Name: "Composite", Kernel: "Composite", Source: "nvme · 0e:00", MilliC: 48000, Series: "temp.Composite.nvme · 0e:00"},
		{Name: "Remote", Kernel: "temp2", Source: "lm75", Fault: true, Series: "temp.temp2"},
	})
	want := map[string][]float64{
		"temp.Composite.nvme · 0f:00": {46},
		"temp.Composite.nvme · 0e:00": {48},
		"temp.cluster-thermal":        {80.5},
	}
	if len(got) != len(want) {
		t.Fatalf("series = %v, want %v", got, want)
	}
	for series, values := range want {
		if !slices.Equal(got[series], values) {
			t.Fatalf("series = %v, want %v", got, want)
		}
	}
}

// TestAStartKeepsWhatWasRecordedUnderTheOldNames: the history a shell
// recorded under a reading's display name moves to the kernel's name for it
// when the history opens, and starting again moves nothing more.
func TestAStartKeepsWhatWasRecordedUnderTheOldNames(t *testing.T) {
	store, err := history.Open(history.Memory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	at := time.Unix(1_800_000_000, 0).Truncate(time.Minute)
	store.Add(history.Point{Series: "temp.Memory", Time: at, Average: 47, Peak: 48})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	temps := []sensors.TempReading{{Name: "Memory", Kernel: "ddr-thermal", Source: "thermal zone", Kind: "zone", MilliC: 47000}}

	for range 2 {
		keepTemperatureHistory(context.Background(), store, temps)
	}
	moved, err := store.Points(context.Background(), "temp.ddr-thermal", history.Minute, at, at.Add(time.Minute))
	if err != nil || len(moved) != 1 || moved[0].Average != 47 {
		t.Fatalf("temp.ddr-thermal holds %+v (%v), want the point recorded as temp.Memory", moved, err)
	}
	if old, _ := store.Points(context.Background(), "temp.Memory", history.Minute, at, at.Add(time.Minute)); len(old) != 0 {
		t.Fatalf("temp.Memory still holds %+v", old)
	}
}

// TestGaugeSkipsWhatCannotBeRead: a reading that fails leaves a gap, never a
// zero.
func TestGaugeSkipsWhatCannotBeRead(t *testing.T) {
	source := gauges(map[string]func() (float64, error){
		"cpu":       func() (float64, error) { return 12.5, nil },
		"conntrack": func() (float64, error) { return 0, errors.New("absent") },
	})
	got := source(time.Unix(0, 0))
	if len(got) != 1 || !slices.Equal(got["cpu"], []float64{12.5}) {
		t.Fatalf("gauges = %v, want only cpu", got)
	}
}

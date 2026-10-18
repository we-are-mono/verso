// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
)

// firstFrames reads the overview stream's first tick under a layout cookie:
// every frame sent, by event name, up to and including the overview frame.
func firstFrames(t *testing.T, srv *Server, layout string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: srv.sessions.CreateWithMetadata("test-sid", "root", "", "")})
	if layout != "" {
		req.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: layout})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	frames, event := map[string]string{}, ""
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if v, ok := strings.CutPrefix(line, "event: "); ok {
			event = v
			continue
		}
		if payload, ok := strings.CutPrefix(line, "data: "); ok {
			frames[event] = payload
			if event == "overview" {
				return frames
			}
		}
	}
	t.Fatalf("stream ended before the overview frame: %v", scanner.Err())
	return nil
}

func streamServer(t *testing.T) *Server {
	backend := metersBackend()
	backend.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "eth0", Uptime: 60, Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Main: true}}}}}
	backend.uci = map[string]map[string]any{"network": {
		"lan": map[string]any{".type": "interface", ".name": "lan", "device": "eth1", "proto": "static"},
	}}
	srv := newServer(t, backend)
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{TimestampMS: uint64(time.Now().UnixMilli()), Interfaces: []telemetry.Interface{
		{Name: "eth0", Physical: true, Kind: "port", Operstate: "up"},
		{Name: "eth1", Physical: true, Kind: "port", Operstate: "down"},
	}}}
	return srv
}

func sent(frames map[string]string) []string {
	var names []string
	for name := range frames {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestOverviewStreamSendsWhatTheShownCardsRead(t *testing.T) {
	all := []string{"clock", "interfaces", "meters", "overview", "wan"}
	for _, tc := range []struct {
		name, layout string
		want         []string
	}{
		{"default", "", all},
		{"everything closed", "1.internet-firewall-ports-tunnels.traffic-resources-system.internet-firewall-ports-tunnels-traffic-resources-system.", all},
		{"Resources hidden", "1.internet-firewall-ports-tunnels.traffic-system..resources", []string{"clock", "interfaces", "overview", "wan"}},
		{"System hidden", "1.internet-firewall-ports-tunnels.traffic-resources..system", []string{"clock", "interfaces", "meters", "overview", "wan"}},
		{"Traffic and Internet hidden", "1.firewall-ports-tunnels.resources-system..internet-traffic", []string{"clock", "interfaces", "meters", "overview"}},
		{"Ports and Tunnels hidden", "1.internet-firewall.traffic-resources-system..ports-tunnels", []string{"clock", "meters", "overview", "wan"}},
		{"everything hidden", "1....internet-firewall-ports-tunnels-traffic-resources-system", []string{"clock", "overview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sent(firstFrames(t, streamServer(t), tc.layout)); !slices.Equal(got, tc.want) {
				t.Errorf("frames = %v, want %v", got, tc.want)
			}
		})
	}
}

// A hidden card's reading still reaches the verdict, so hiding a card never
// makes the page read "online": the heading names the hidden card's cause.
func TestOverviewStreamVerdictCountsHiddenCards(t *testing.T) {
	frames := firstFrames(t, streamServer(t), "1.internet-firewall.traffic-resources-system..ports-tunnels")
	var status widget.OverviewStatus
	if err := json.Unmarshal([]byte(frames["overview"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Tone != "warning" || status.Lead != "1 port without link." {
		t.Fatalf("verdict = %+v, want the hidden Ports card's lost link counted", status)
	}
	for _, card := range status.Cards {
		if card.ID == "ports" && (card.Tone != "warning" || card.Status != "1 port without link") {
			t.Errorf("ports = %+v, want the topology the first paint has", card)
		}
	}
}

// The stream's card heads are the first paint's: a port a network uses is
// still a port in use one second later.
func TestOverviewStreamKeepsTheFirstPaintsTopology(t *testing.T) {
	frames := firstFrames(t, streamServer(t), "")
	var status widget.OverviewStatus
	if err := json.Unmarshal([]byte(frames["overview"]), &status); err != nil {
		t.Fatal(err)
	}
	for _, card := range status.Cards {
		if card.ID == "ports" && card.Status != "1 port without link" {
			t.Errorf("ports = %+v", card)
		}
	}
}

// A connection's first WAN frame carries the uplink's minute as the router
// sampled it, so a graph that was away (a hidden tab, a dropped stream) takes
// the minute up again where it truly stands instead of gluing the next
// sample onto the one it saw last.
func TestOverviewStreamOpensWithTheUplinksMinute(t *testing.T) {
	srv := streamServer(t)
	now := uint64(time.Now().UnixMilli())
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{TimestampMS: now, Interfaces: []telemetry.Interface{
		{Name: "eth0", Physical: true, Kind: "port", Operstate: "up", History: []telemetry.Point{
			{TimestampMS: now - 2000, RxBPS: 10e6, TxBPS: 1e6},
			{TimestampMS: now - 1000, RxBPS: 20e6, TxBPS: 2e6},
		}},
	}}}
	var wan struct {
		Down, Up float64
		Minute   *struct{ Down, Up []float64 }
	}
	if err := json.Unmarshal([]byte(firstFrames(t, srv, "")["wan"]), &wan); err != nil {
		t.Fatal(err)
	}
	if wan.Minute == nil {
		t.Fatal("the first WAN frame carries no minute")
	}
	m := wan.Minute
	if len(m.Down) != telemetrySeriesLen || len(m.Up) != telemetrySeriesLen {
		t.Fatalf("minute = %d/%d points, want the full %d", len(m.Down), len(m.Up), telemetrySeriesLen)
	}
	if got := [2]float64{m.Down[len(m.Down)-2], m.Down[len(m.Down)-1]}; got != [2]float64{10, 20} || m.Up[len(m.Up)-1] != 2 {
		t.Errorf("minute ends %v down, %v up; want the sampler's last two seconds", got, m.Up[len(m.Up)-1])
	}
	if wan.Down != 20 || wan.Up != 2 {
		t.Errorf("now = %v/%v, want the minute's newest", wan.Down, wan.Up)
	}
}

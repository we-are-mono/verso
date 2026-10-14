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
	return framesUntil(t, srv, "/overview/events", layout, "overview")
}

// framesUntil reads a stream from path until the frame named last arrives:
// every frame sent before it, and it, by event name.
func framesUntil(t *testing.T, srv *Server, path, layout, last string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+path, nil)
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
			if event == last {
				return frames
			}
		}
	}
	t.Fatalf("stream ended before the %s frame: %v", last, scanner.Err())
	return nil
}

// TestAPageReadsOnlyThePortsFrame: a page that draws the router's panel and
// nothing else of the Overview (System › Hardware) names the frame it reads,
// and the stream sends that one alone, whatever the Overview's layout hides;
// a frame outside the allowlist is never sent this way.
func TestAPageReadsOnlyThePortsFrame(t *testing.T) {
	hidden := "1.internet-firewall.traffic-resources-hardware-firmware..ports-tunnels"
	for _, path := range []string{"/overview/events?reads=interfaces", "/overview/events?reads=interfaces,meters,clock"} {
		frames := framesUntil(t, streamServer(t), path, hidden, "interfaces")
		if got := sent(frames); !slices.Equal(got, []string{"interfaces"}) {
			t.Errorf("%s: frames = %v, want only interfaces", path, got)
		}
		if !strings.Contains(frames["interfaces"], `"linked":true`) {
			t.Errorf("%s: the frame should carry the ports' link: %s", path, frames["interfaces"])
		}
	}
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
	all := []string{"clock", "interfaces", "meters", "overview", "sensors", "wan"}
	for _, tc := range []struct {
		name, layout string
		want         []string
	}{
		{"default", "", all},
		{"everything closed", "1.internet-firewall-ports-tunnels.traffic-resources-hardware-firmware.internet-firewall-ports-tunnels-traffic-resources-hardware-firmware.", all},
		{"Resources hidden", "1.internet-firewall-ports-tunnels.traffic-hardware-firmware..resources", []string{"clock", "interfaces", "overview", "sensors", "wan"}},
		{"Hardware hidden", "1.internet-firewall-ports-tunnels.traffic-resources-firmware..hardware", []string{"clock", "interfaces", "meters", "overview", "wan"}},
		{"Traffic and Internet hidden", "1.firewall-ports-tunnels.resources-hardware-firmware..internet-traffic", []string{"clock", "interfaces", "meters", "overview", "sensors"}},
		{"Ports and Tunnels hidden", "1.internet-firewall.traffic-resources-hardware-firmware..ports-tunnels", []string{"clock", "meters", "overview", "sensors", "wan"}},
		{"everything hidden", "1....internet-firewall-ports-tunnels-traffic-resources-hardware-firmware", []string{"clock", "overview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sent(firstFrames(t, streamServer(t), tc.layout)); !slices.Equal(got, tc.want) {
				t.Errorf("frames = %v, want %v", got, tc.want)
			}
		})
	}
}

// A hidden card's reading still reaches the verdict, so hiding a card never
// makes the page read "online".
func TestOverviewStreamVerdictCountsHiddenCards(t *testing.T) {
	frames := firstFrames(t, streamServer(t), "1.internet-firewall.traffic-resources-hardware-firmware..ports-tunnels")
	var status widget.OverviewStatus
	if err := json.Unmarshal([]byte(frames["overview"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Tone != "warning" || status.Lead != "Your network needs attention" {
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

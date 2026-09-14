// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
)

func TestOverviewSlovenianPageAndStream(t *testing.T) {
	bundle, problems := i18n.Load(os.DirFS("../../i18n"), "*/*.json")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	backend := metersBackend()
	backend.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "wan-live", Uptime: 8040, Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Main: true}}}}}
	backend.wanConn = openwrt.WANConn{V4Proto: "DHCP", V4Addr: "192.0.2.1/24", V4Gateway: "192.0.2.254", V6Proto: "DHCPv6 client", V6Prefix: "2001:db8::/56", V6Valid: 480}
	srv := newServer(t, backend)
	srv.SetBundle(bundle)
	srv.stats = fakeStats{cpu: 91, root: sysstat.Storage{Used: 95, Free: 5}}
	srv.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{TimestampMS: uint64(time.Now().UnixMilli()), Interfaces: []telemetry.Interface{{Name: "eth0", Physical: true, Kind: "port", Operstate: "down"}, {Name: "wg0", Kind: "tunnel", Operstate: "unknown"}}}}
	body := getLang(t, srv, "/", "sl-SI")
	for _, want := range []string{"Vaše omrežje zahteva pozornost", "Požarni zid", "Tuneli", "Predpona", "Prehod", "Velja še", "8 min", "Pomnilnik", "Mbit/s prejem", "Mbit/s oddaja", "pred 60 s", "že 2 h 14 min", "192.0.2.1/24", "2001:db8::/56"} {
		if !strings.Contains(body, want) {
			t.Errorf("Slovenian overview missing %q", want)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	req.Header.Set("Accept-Language", "sl-SI")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	event := ""
	seen := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if v, ok := strings.CutPrefix(line, "event: "); ok {
			event = v
			continue
		}
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		seen[event] = true
		switch event {
		case "meters":
			meters := decodeMeters(t, payload)
			if meters[0].Label != "Obremenitev" || !strings.Contains(meters[0].Detail, "1-minutno povprečje") {
				t.Errorf("untranslated meters: %+v", meters)
			}
			if meters[1].Band != "danger" || meters[1].Role != "" {
				t.Errorf("CPU should keep critical band in stream: %+v", meters[1])
			}
		case "interfaces":
			if !strings.Contains(payload, "Ne deluje") || !strings.Contains(payload, "Neznano") {
				t.Errorf("untranslated state: %s", payload)
			}
		case "wan":
			if !strings.Contains(payload, "že 2 h 14 min") {
				t.Errorf("untranslated WAN duration: %s", payload)
			}
		case "overview":
			var status widget.OverviewStatus
			if err := json.Unmarshal([]byte(payload), &status); err != nil {
				t.Fatal(err)
			}
			if status.Lead != "Vaše omrežje zahteva pozornost" || status.Tone != "warning" {
				t.Errorf("stream verdict=%+v", status)
			}
			for _, required := range []string{"clock", "meters", "interfaces", "wan"} {
				if !seen[required] {
					t.Errorf("missing %s frame", required)
				}
			}
			return
		}
	}
	t.Fatalf("stream ended before overview frame: %v", scanner.Err())
}

func TestSystemMeterDesignThresholds(t *testing.T) {
	for _, limits := range [][2]int{{60, 85}, {70, 90}, {75, 90}, {80, 90}} {
		for _, tc := range []struct {
			fill int
			want string
		}{{limits[0] - 1, "success"}, {limits[0], "warning"}, {limits[1] - 1, "warning"}, {limits[1], "danger"}} {
			if got := systemMeterBand(tc.fill, limits[0], limits[1]); got != tc.want {
				t.Errorf("limits %v fill %d=%s", limits, tc.fill, got)
			}
		}
	}
}

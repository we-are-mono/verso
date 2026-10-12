// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

type firewallOverviewBackend struct {
	fakeBackend
	mu     sync.Mutex
	status openwrt.FirewallStatus
	err    error
	sid    string
}

func (b *firewallOverviewBackend) FirewallStatus(_ context.Context, sid string) (openwrt.FirewallStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sid = sid
	return b.status, b.err
}

// No card shows the firewall; its runtime state reaches the verdict, on the
// first paint and live, and a failed read clears the old state rather than
// leaving it standing.
func TestOverviewFirewallRefreshesAndClearsStaleState(t *testing.T) {
	backend := &firewallOverviewBackend{status: openwrt.FirewallStatus{State: "inactive"}}
	backend.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "eth0", Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Main: true}}}}}
	srv := newServer(t, backend)
	srv.eventInterval = 10 * time.Millisecond
	// Calm resources and no interfaces, so the firewall is the only thing the
	// verdict weighs.
	srv.stats = fakeStats{cpu: 5, root: sysstat.Storage{Used: 10, Free: 90}}
	srv.telemetry = fakeTelemetry{}
	if body := get(t, srv, "/").Body.String(); !strings.Contains(body, "Your network needs attention") {
		t.Fatal("first paint did not count the inactive firewall")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	token := srv.sessions.CreateWithMetadata("firewall-reader", "root", "", "")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for _, want := range []struct {
		state, tone string
		err         error
	}{
		{"active", "success", nil},
		{"inactive", "warning", nil},
		{"partial", "warning", nil},
		{"inactive", "success", errors.New("read failed")},
	} {
		backend.mu.Lock()
		backend.status, backend.err = openwrt.FirewallStatus{State: want.state}, want.err
		backend.mu.Unlock()
		matched, event, last := false, "", ""
		for !matched && scanner.Scan() {
			line := scanner.Text()
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				event = v
				continue
			}
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok || event != "overview" {
				continue
			}
			last = payload
			var status widget.OverviewStatus
			if err := json.Unmarshal([]byte(payload), &status); err != nil {
				t.Fatal(err)
			}
			matched = status.Tone == want.tone
		}
		if !matched {
			t.Fatalf("stream never reached a %s verdict for %s (last %s): %v", want.tone, want.state, last, scanner.Err())
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.sid != "firewall-reader" {
		t.Fatalf("read used sid %q", backend.sid)
	}
}

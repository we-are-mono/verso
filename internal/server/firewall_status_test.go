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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

type firewallOverviewBackend struct {
	fakeBackend
	mu        sync.Mutex
	status    openwrt.FirewallStatus
	err       error
	sid       string
	config    map[string]any
	configErr error
}

func (b *firewallOverviewBackend) FirewallStatus(_ context.Context, sid string) (openwrt.FirewallStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sid = sid
	return b.status, b.err
}

func (b *firewallOverviewBackend) UCIConfig(ctx context.Context, sid, config string) (map[string]any, error) {
	if config != "firewall" {
		return b.fakeBackend.UCIConfig(ctx, sid, config)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sid = sid
	return b.config, b.configErr
}

func firewallRuleConfig(n int) map[string]any {
	config := map[string]any{}
	for _, kind := range []string{"defaults", "zone", "forwarding", "redirect", "nat", "include"} {
		config[kind] = map[string]any{".type": kind}
	}
	for i := 0; i < n; i++ {
		// Disabled rules remain rows on the Firewall page and belong in this
		// configured count; they say nothing about which rules are loaded.
		config["rule"+strconv.Itoa(i)] = map[string]any{".type": "rule", "enabled": strconv.Itoa(i % 2)}
	}
	return config
}

func TestOverviewFirewallRefreshesAndClearsStaleState(t *testing.T) {
	backend := &firewallOverviewBackend{status: openwrt.FirewallStatus{State: "active", Rules: 65}, config: firewallRuleConfig(7)}
	srv := newServer(t, backend)
	srv.eventInterval = 10 * time.Millisecond
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, "7 configured rules") || strings.Contains(body, "65 rules") {
		t.Fatal("first paint did not read the firewall")
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
	for _, want := range []struct {
		state, status, tone, caption string
		rules                        int
		err, configErr               error
	}{
		{"active", "Active", "success", "7 configured rules", 7, nil, nil},
		{"inactive", "Inactive", "danger", "Firewall is not running", 7, nil, nil},
		{"active", "Status unavailable", "neutral", "", 7, errors.New("read failed"), nil},
		{"active", "Active", "success", "Ruleset loaded", 7, nil, errors.New("config unavailable")},
		{"active", "Active", "success", "0 configured rules", 0, nil, nil},
		{"active", "Active", "success", "2 configured rules", 2, nil, nil},
	} {
		backend.mu.Lock()
		backend.status, backend.err = openwrt.FirewallStatus{State: want.state, Rules: 65}, want.err
		backend.config, backend.configErr = firewallRuleConfig(want.rules), want.configErr
		backend.mu.Unlock()
		matched, event := false, ""
		for scanner.Scan() {
			line := scanner.Text()
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				event = v
				continue
			}
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok || event != "overview" {
				continue
			}
			var status widget.OverviewStatus
			if err := json.Unmarshal([]byte(payload), &status); err != nil {
				t.Fatal(err)
			}
			for _, tile := range status.Tiles {
				if tile.ID != "firewall" || tile.Status != want.status || tile.Tone != want.tone || tile.Caption != want.caption {
					continue
				}
				matched = true
			}
			if matched {
				break
			}
		}
		if !matched {
			t.Fatalf("stream never reached %s: %v", want.status, scanner.Err())
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.sid != "firewall-reader" {
		t.Fatalf("read used sid %q", backend.sid)
	}
}

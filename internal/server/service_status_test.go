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

type failedServicesBackend struct {
	fakeBackend
	mu     sync.Mutex
	failed []string
	err    error
	reads  int
}

func (b *failedServicesBackend) FailedServices(context.Context, string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads++
	return b.failed, b.err
}

func (b *failedServicesBackend) set(failed []string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failed, b.err = failed, err
}

// A service procd stopped restarting reaches the verdict on the first paint
// and live, the System card opens the Services page while it stands, and a
// failed read clears it rather than leaving an old failure standing.
func TestOverviewFailedServicesRefreshAndClear(t *testing.T) {
	backend := &failedServicesBackend{failed: []string{"sysntpd"}}
	backend.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "eth0", Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Main: true}}}}}
	backend.fwStatus = openwrt.FirewallStatus{State: "active", Rules: 1}
	srv := newServer(t, backend)
	srv.eventInterval = 10 * time.Millisecond
	srv.stats = fakeStats{cpu: 5, root: sysstat.Storage{Used: 10, Free: 90}}
	srv.telemetry = fakeTelemetry{}
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, "sysntpd failed") || !strings.Contains(body, `href="/system/services"`) {
		t.Fatal("first paint did not name the failed service and open Services")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	token := srv.sessions.CreateWithMetadata("service-reader", "root", "", "")
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
		failed []string
		err    error
		lead   string
	}{
		{nil, nil, ""},
		{[]string{"odhcpd", "sysntpd"}, nil, "2 services failed"},
		{[]string{"sysntpd"}, errors.New("read failed"), ""},
	} {
		backend.set(want.failed, want.err)
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
			if want.lead == "" {
				matched = status.Tone == "success"
			} else {
				matched = status.Lead == want.lead && status.Tone == "danger"
			}
		}
		if !matched {
			t.Fatalf("stream never reached %q for %v (last %s): %v", want.lead, want.failed, last, scanner.Err())
		}
	}
}

// The live stream reads procd's service table once per serviceStatusEvery
// frames, not on every frame.
func TestOverviewStreamReadsServicesSparingly(t *testing.T) {
	backend := &failedServicesBackend{}
	srv := newServer(t, backend)
	srv.eventInterval = 5 * time.Millisecond
	srv.telemetry = fakeTelemetry{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	token := srv.sessions.CreateWithMetadata("service-reader", "root", "", "")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	frames := 0
	for frames < 2*serviceStatusEvery && scanner.Scan() {
		if scanner.Text() == "event: overview" {
			frames++
		}
	}
	cancel()
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.reads == 0 || backend.reads > 3 {
		t.Errorf("%d frames read the service table %d times, want 2 or 3", frames, backend.reads)
	}
}

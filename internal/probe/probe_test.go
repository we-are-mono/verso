// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package probe

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
)

// TestGatewayIsTheMainTableDefaultsNextHop: /proc/net/route lists the main
// table, addresses in host byte order; the default route through a gateway
// names it. A point-to-point uplink (PPPoE) has none.
func TestGatewayIsTheMainTableDefaultsNextHop(t *testing.T) {
	route := []byte("Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth0\t000014AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n" +
		"wan0\t00000000\t02011EAC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n")
	got, ok := gateway(route)
	if !ok || got != netip.MustParseAddr("172.30.1.2") {
		t.Fatalf("gateway = %v, %v; want 172.30.1.2", got, ok)
	}
	pppoe := []byte("Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"pppoe-wan\t00000000\t00000000\t0001\t0\t0\t0\t00000000\t0\t0\t0\n")
	if got, ok := gateway(pppoe); ok {
		t.Fatalf("a point-to-point default named gateway %v", got)
	}
}

// TestRoundRecordsLatencyAndLoss: an answer is its round trip in
// milliseconds and no loss; silence is a loss; a target with no address this
// round (no gateway) and a probe the system refuses to send are gaps, never
// losses.
func TestRoundRecordsLatencyAndLoss(t *testing.T) {
	answers := map[netip.Addr]error{
		netip.MustParseAddr("192.0.2.1"): nil,
		netip.MustParseAddr("192.0.2.2"): ErrTimeout,
		netip.MustParseAddr("192.0.2.3"): ErrNotPermitted,
	}
	p := &Prober{
		ping: func(addr netip.Addr, _ time.Duration) (time.Duration, error) {
			return 12500 * time.Microsecond, answers[addr]
		},
		targets: []Target{
			{Name: "gateway", Addr: func() (netip.Addr, bool) { return netip.MustParseAddr("192.0.2.1"), true }},
			{Name: "192.0.2.2", Addr: fixed(netip.MustParseAddr("192.0.2.2"))},
			{Name: "192.0.2.3", Addr: fixed(netip.MustParseAddr("192.0.2.3"))},
			{Name: "nowhere", Addr: func() (netip.Addr, bool) { return netip.Addr{}, false }},
		},
	}
	p.Round()
	p.Round()
	got := p.Source(time.Now())
	want := map[string][]float64{
		"wan.gateway.latency": {12.5, 12.5},
		"wan.gateway.loss":    {0, 0},
		"wan.192.0.2.2.loss":  {100, 100},
	}
	if len(got) != len(want) {
		t.Fatalf("readings = %v, want %v", got, want)
	}
	for series, values := range want {
		if !slices.Equal(got[series], values) {
			t.Fatalf("readings = %v, want %v", got, want)
		}
	}
	if again := p.Source(time.Now()); len(again) != 0 {
		t.Fatalf("readings handed over twice: %v", again)
	}
}

// TestPingAnswersFromLoopback: a real echo over an unprivileged ping socket.
// Skipped where this user's group may not open one.
func TestPingAnswersFromLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "::1"} {
		rtt, err := Ping(netip.MustParseAddr(addr), time.Second)
		if errors.Is(err, ErrNotPermitted) {
			t.Skipf("ping sockets not permitted here: %v", err)
		}
		if err != nil || rtt <= 0 || rtt > time.Second {
			t.Fatalf("Ping(%s) = %v, %v", addr, rtt, err)
		}
	}
}

// TestPingTimesOutOnSilence: an address that never answers is a timeout, not
// a hang.
func TestPingTimesOutOnSilence(t *testing.T) {
	start := time.Now()
	_, err := Ping(netip.MustParseAddr("192.0.2.1"), 200*time.Millisecond)
	if errors.Is(err, ErrNotPermitted) {
		t.Skip("ping sockets not permitted here")
	}
	if err == nil {
		t.Fatal("TEST-NET-1 answered")
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("waited %v for a 200 ms timeout", waited)
	}
}

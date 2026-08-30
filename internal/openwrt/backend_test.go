// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"testing"
)

func fakeSystemInfo(m map[string]any, err error) systemInfoFn {
	return func(context.Context, string) (map[string]any, error) { return m, err }
}

func TestSystemInfoMapsFields(t *testing.T) {
	m := map[string]any{
		"uptime": int64(1287),
		"load":   []any{int64(1566912), int64(465440), int64(189920)},
		"memory": map[string]any{
			"total":     int64(64883740672),
			"free":      int64(22826393600),
			"available": int64(37969338368),
		},
	}
	b := &NativeBackend{systemInfo: fakeSystemInfo(m, nil)}

	si, err := b.SystemInfo(context.Background(), "sid")
	if err != nil {
		t.Fatalf("SystemInfo: %v", err)
	}
	if si.Uptime != 1287 {
		t.Errorf("Uptime = %d, want 1287", si.Uptime)
	}
	if si.Load[0] != 1566912 {
		t.Errorf("Load[0] = %d, want 1566912", si.Load[0])
	}
	if si.Memory.Total != 64883740672 || si.Memory.Available != 37969338368 {
		t.Errorf("Memory = %+v", si.Memory)
	}
}

func TestBoardPrefersFullReleaseAndKernelBuild(t *testing.T) {
	b := &NativeBackend{
		systemBoard: func(context.Context, string) (map[string]any, error) {
			return map[string]any{
				"kernel": "6.12.101",
				"release": map[string]any{
					"distribution": "OpenWrt", "version": "25.12.4",
					"description": "OpenWrt 25.12.4 r32933-4ccb782af7",
					"target":      "qualcommax/ipq807x",
				},
			}, nil
		},
		kernelBuild: func() string { return "Linux version 6.12.101 (builder@host) #1 SMP" },
	}
	got, err := b.Board(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Firmware != "OpenWrt 25.12.4 r32933-4ccb782af7" {
		t.Errorf("firmware = %q", got.Firmware)
	}
	if got.KernelBuild != "Linux version 6.12.101 (builder@host) #1 SMP" {
		t.Errorf("kernel build = %q", got.KernelBuild)
	}
	if got.Target != "qualcommax/ipq807x" {
		t.Errorf("target = %q", got.Target)
	}
}

func TestSystemInfoError(t *testing.T) {
	b := &NativeBackend{systemInfo: fakeSystemInfo(nil, errors.New("boom"))}
	if _, err := b.SystemInfo(context.Background(), "sid"); err == nil {
		t.Fatal("SystemInfo: want error, got nil")
	}
}

// TestHostnamePassesSession checks the seam is called and the sid is threaded
// through to it — the real read goes through rpcd's uci object (verified live).
func TestHostnamePassesSession(t *testing.T) {
	var gotSID string
	b := &NativeBackend{hostname: func(_ context.Context, sid string) (string, error) {
		gotSID = sid
		return "verso-lab", nil
	}}

	hn, err := b.Hostname(context.Background(), "s1")
	if err != nil || hn != "verso-lab" {
		t.Fatalf("Hostname = %q, %v; want verso-lab", hn, err)
	}
	if gotSID != "s1" {
		t.Errorf("sid not threaded to the backend: got %q", gotSID)
	}
}

// TestAccessPassesProbe checks Access threads the full ACL triple through to the
// seam — the real probe asks rpcd's session.access (verified live). This is the
// enforcement primitive the shell uses to gate plugin writes (ADR-007).
func TestAccessPassesProbe(t *testing.T) {
	var got [4]string
	b := &NativeBackend{access: func(_ context.Context, sid, scope, object, function string) (bool, error) {
		got = [4]string{sid, scope, object, function}
		return true, nil
	}}

	ok, err := b.Access(context.Background(), "s1", "uci", "system", "write")
	if err != nil || !ok {
		t.Fatalf("Access = %v, %v; want true, nil", ok, err)
	}
	if got != [4]string{"s1", "uci", "system", "write"} {
		t.Errorf("probe args = %v, want [s1 uci system write]", got)
	}
}

func TestSetSystemTimeThreadsStructuredArgs(t *testing.T) {
	var got [3]string
	b := &NativeBackend{setTime: func(_ context.Context, sid, datetime, timezone string) error {
		got = [3]string{sid, datetime, timezone}
		return nil
	}}
	if err := b.SetSystemTime(context.Background(), "s1", "2026-08-30T12:34:56", "GMT0"); err != nil {
		t.Fatalf("SetSystemTime: %v", err)
	}
	if got != [3]string{"s1", "2026-08-30T12:34:56", "GMT0"} {
		t.Errorf("args = %v", got)
	}
}

// TestUCISetThreadsArgs checks UCISet passes the sid, config, section and values
// to the seam — the real write brokers rpcd's uci.set carrying the operator's sid
// (ADR-007: the shell writes on the plugin's behalf).
func TestUCISetThreadsArgs(t *testing.T) {
	var gotSID, gotConfig, gotSection string
	var gotValues map[string]any
	b := &NativeBackend{uciSet: func(_ context.Context, sid, config, section string, values map[string]any) error {
		gotSID, gotConfig, gotSection, gotValues = sid, config, section, values
		return nil
	}}

	err := b.UCISet(context.Background(), "s1", "system", "@system[0]", map[string]any{"hostname": "verso-lab"})
	if err != nil {
		t.Fatalf("UCISet: %v", err)
	}
	if gotSID != "s1" || gotConfig != "system" || gotSection != "@system[0]" || gotValues["hostname"] != "verso-lab" {
		t.Errorf("args not threaded: sid=%q config=%q section=%q values=%v", gotSID, gotConfig, gotSection, gotValues)
	}
}

// TestUCICommitThreadsArgs checks UCICommit passes the sid and config through.
func TestUCICommitThreadsArgs(t *testing.T) {
	var gotSID, gotConfig string
	b := &NativeBackend{uciCommit: func(_ context.Context, sid, config string) error {
		gotSID, gotConfig = sid, config
		return nil
	}}

	if err := b.UCICommit(context.Background(), "s1", "system"); err != nil {
		t.Fatalf("UCICommit: %v", err)
	}
	if gotSID != "s1" || gotConfig != "system" {
		t.Errorf("args not threaded: sid=%q config=%q", gotSID, gotConfig)
	}
}

// TestUCIConfigThreadsArgs checks UCIConfig passes the sid and config through and
// returns the whole-config values map the shell hands a plugin as its read
// snapshot (ADR-007).
func TestUCIConfigThreadsArgs(t *testing.T) {
	var gotSID, gotConfig string
	want := map[string]any{"wg0": map[string]any{".type": "interface", "proto": "wireguard"}}
	b := &NativeBackend{uciConfig: func(_ context.Context, sid, config string) (map[string]any, error) {
		gotSID, gotConfig = sid, config
		return want, nil
	}}

	got, err := b.UCIConfig(context.Background(), "s1", "network")
	if err != nil {
		t.Fatalf("UCIConfig: %v", err)
	}
	if gotSID != "s1" || gotConfig != "network" {
		t.Errorf("args not threaded: sid=%q config=%q", gotSID, gotConfig)
	}
	if _, ok := got["wg0"]; !ok {
		t.Errorf("UCIConfig result = %v, want the wg0 section", got)
	}
}

// TestUCIAddThreadsArgsAndReturnsSection checks UCIAdd passes sid/config/type
// through and returns rpcd's new section id — the shell realizes a repeater's "add"
// (ADR-005 §7).
func TestUCIAddThreadsArgsAndReturnsSection(t *testing.T) {
	var gotSID, gotConfig, gotType string
	b := &NativeBackend{uciAdd: func(_ context.Context, sid, config, secType string) (string, error) {
		gotSID, gotConfig, gotType = sid, config, secType
		return "cfg123", nil
	}}

	sec, err := b.UCIAdd(context.Background(), "s1", "network", "wireguard_wg0")
	if err != nil {
		t.Fatalf("UCIAdd: %v", err)
	}
	if gotSID != "s1" || gotConfig != "network" || gotType != "wireguard_wg0" || sec != "cfg123" {
		t.Errorf("args/return wrong: sid=%q config=%q type=%q sec=%q", gotSID, gotConfig, gotType, sec)
	}
}

// TestUCIDeleteThreadsArgs checks UCIDelete passes sid/config/section through.
func TestUCIDeleteThreadsArgs(t *testing.T) {
	var gotSID, gotConfig, gotSection string
	b := &NativeBackend{uciDelete: func(_ context.Context, sid, config, section string) error {
		gotSID, gotConfig, gotSection = sid, config, section
		return nil
	}}

	if err := b.UCIDelete(context.Background(), "s1", "network", "@wireguard_wg0[0]"); err != nil {
		t.Fatalf("UCIDelete: %v", err)
	}
	if gotSID != "s1" || gotConfig != "network" || gotSection != "@wireguard_wg0[0]" {
		t.Errorf("args not threaded: sid=%q config=%q section=%q", gotSID, gotConfig, gotSection)
	}
}

func TestParseWANConnIPv6(t *testing.T) {
	v4 := map[string]any{
		"proto": "dhcp",
		"ipv4-address": []any{
			map[string]any{"address": "10.0.0.138", "mask": int64(24)},
		},
	}
	v6 := map[string]any{
		"proto": "dhcpv6",
		"ipv6-address": []any{
			map[string]any{"address": "2001:db8::f7c", "mask": int64(128)},
		},
		"ipv6-prefix": []any{
			map[string]any{"address": "2001:db8:1::", "mask": int64(56), "valid": int64(690)},
		},
		"route": []any{
			map[string]any{"target": "::", "mask": int64(0), "nexthop": "fe80::1"},
		},
		"dns-server": []any{"2001:4860:4860::8888"},
	}

	got := parseWANConn(v4, v6)
	if got.V6Proto != "DHCPv6 client" || got.V6Addr != "2001:db8::f7c/128" ||
		got.V6Prefix != "2001:db8:1::/56" || got.V6Gateway != "fe80::1" ||
		len(got.V6DNS) != 1 || got.V6DNS[0] != "2001:4860:4860::8888" || got.V6Valid != 690 {
		t.Errorf("wan6 facts = %+v", got)
	}

	// Some netifd protocols expose the IPv6 facts on wan itself instead of
	// creating a separate wan6 object.
	dualStack := map[string]any{}
	for key, value := range v4 {
		dualStack[key] = value
	}
	for key, value := range v6 {
		dualStack[key] = value
	}
	got = parseWANConn(dualStack, nil)
	if got.V6Addr != "2001:db8::f7c/128" || got.V6Prefix != "2001:db8:1::/56" {
		t.Errorf("wan fallback facts = %+v", got)
	}
}

func TestWANStatusesUsesRoutesRatherThanNames(t *testing.T) {
	dump := map[string]any{"interface": []any{
		map[string]any{
			"interface": "lan6", "proto": "dhcpv6", "l3_device": "br-lan",
			"ipv6-address": []any{map[string]any{"address": "fd00::1"}},
		},
		map[string]any{
			"interface": "upstream", "up": true, "proto": "pppoe", "l3_device": "pppoe-upstream",
			"ipv4-address": []any{map[string]any{"address": "192.0.2.2"}},
			"route":        []any{map[string]any{"target": "0.0.0.0", "mask": int64(0), "metric": int64(20)}},
		},
		map[string]any{
			"interface": "upstream_6", "up": true, "proto": "dhcpv6", "dynamic": true,
			"l3_device":   "pppoe-upstream",
			"ipv6-prefix": []any{map[string]any{"address": "2001:db8:1::", "mask": int64(56)}},
			"route":       []any{map[string]any{"target": "::", "mask": int64(0), "metric": int64(20)}},
		},
	}}

	wan, wan6 := wanStatuses(dump)
	if name, _ := wan["interface"].(string); name != "upstream" {
		t.Fatalf("wan interface = %q", name)
	}
	if name, _ := wan6["interface"].(string); name != "upstream_6" {
		t.Fatalf("IPv6 companion = %q, want upstream_6", name)
	}
}

func TestWANStatusesAllowsIPv6DirectlyOnWAN(t *testing.T) {
	wan := map[string]any{
		"interface": "upstream", "up": true, "proto": "pppoe", "l3_device": "pppoe-upstream",
		"ipv6-address": []any{map[string]any{"address": "2001:db8::2"}},
		"route": []any{
			map[string]any{"target": "0.0.0.0", "mask": int64(0)},
			map[string]any{"target": "::", "mask": int64(0)},
		},
	}
	gotWAN, gotV6 := wanStatuses(map[string]any{"interface": []any{wan}})
	if gotWAN == nil || gotV6 == nil || gotWAN["interface"] != gotV6["interface"] {
		t.Fatalf("statuses = wan:%v v6:%v, want one dual-stack owner", gotWAN, gotV6)
	}
	if got := parseWANConn(gotWAN, gotV6).V6Addr; got != "2001:db8::2" {
		t.Errorf("direct WAN IPv6 = %q", got)
	}
}

// TestParseDeviceStats: netifd's "1000F" speed string reads as Mbps, counters
// come from the statistics table, and an unknown speed maps to 0.
func TestParseDeviceStats(t *testing.T) {
	got := parseDeviceStats(map[string]any{
		"carrier": true, "speed": "1000F",
		"statistics": map[string]any{"rx_bytes": int64(7733), "tx_bytes": int64(13877)},
	})
	if !got.Carrier || got.SpeedMbps != 1000 || got.RxBytes != 7733 || got.TxBytes != 13877 {
		t.Errorf("device stats = %+v", got)
	}
	if got := parseDeviceStats(map[string]any{"speed": "-1"}); got.SpeedMbps != 0 {
		t.Errorf("unknown speed = %d, want 0", got.SpeedMbps)
	}
}

// TestParseV6Leases folds the odhcpd ipv6leases response — per-device, per-lease
// with an ipv6-addr array — to a flat DUID/hostname/addresses list, skipping any
// lease without a DUID.
func TestParseV6Leases(t *testing.T) {
	m := map[string]any{
		"device": map[string]any{
			"br-lan": map[string]any{
				"leases": []any{
					map[string]any{
						"duid":     "00030001a483e72b190c",
						"hostname": "gaming-pc",
						"ipv6-addr": []any{
							map[string]any{"address": "2001:db8::4f", "valid": 43200.0},
							map[string]any{"address": "fd00::4f"},
						},
					},
					map[string]any{"hostname": "no-duid"}, // skipped: no DUID
				},
			},
		},
	}
	got := parseV6Leases(m)
	if len(got) != 1 {
		t.Fatalf("leases = %d, want 1 (the DUID-less one dropped): %+v", len(got), got)
	}
	l := got[0]
	if l.DUID != "00030001a483e72b190c" || l.Hostname != "gaming-pc" {
		t.Errorf("lease id wrong: %+v", l)
	}
	if len(l.Addrs) != 2 || l.Addrs[0] != "2001:db8::4f" || l.Addrs[1] != "fd00::4f" {
		t.Errorf("lease addrs wrong: %+v", l.Addrs)
	}
}

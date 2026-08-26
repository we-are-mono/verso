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

// TestParseWANState: up plus the l3 device once the protocol holds it, the
// configured device as fallback, and a down interface reads honestly down.
func TestParseWANState(t *testing.T) {
	got := parseWANState(map[string]any{"up": true, "l3_device": "wan0", "device": "wan0"})
	if !got.Up || got.Device != "wan0" {
		t.Errorf("up wan = %+v", got)
	}
	got = parseWANState(map[string]any{"up": false, "device": "wan0"})
	if got.Up || got.Device != "wan0" {
		t.Errorf("down wan = %+v", got)
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

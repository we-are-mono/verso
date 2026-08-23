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
// (ADR-007 Model B: the shell writes on the plugin's behalf).
func TestUCISetThreadsArgs(t *testing.T) {
	var gotSID, gotConfig, gotSection string
	var gotValues map[string]string
	b := &NativeBackend{uciSet: func(_ context.Context, sid, config, section string, values map[string]string) error {
		gotSID, gotConfig, gotSection, gotValues = sid, config, section, values
		return nil
	}}

	err := b.UCISet(context.Background(), "s1", "system", "@system[0]", map[string]string{"hostname": "verso-lab"})
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

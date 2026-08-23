// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHostnameFromUCI(t *testing.T) {
	b := &NativeBackend{uciDir: "testdata/config"}

	hn, err := b.Hostname(context.Background())
	if err != nil {
		t.Fatalf("Hostname: %v", err)
	}
	if hn != "verso-lab" {
		t.Errorf("Hostname = %q, want %q", hn, "verso-lab")
	}
}

func TestHostnameMissing(t *testing.T) {
	b := &NativeBackend{uciDir: "testdata/empty"}
	if _, err := b.Hostname(context.Background()); err == nil {
		t.Fatal("Hostname: want error when config missing, got nil")
	}
}

// TestHostnameReflectsExternalWrite guards the fresh-read fix: a change committed
// to the config by another process (e.g. the hostname plugin) must be visible on
// the next read, not masked by a tree cached at startup.
func TestHostnameReflectsExternalWrite(t *testing.T) {
	dir := t.TempDir()
	writeHostname := func(hn string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "system"),
			[]byte("config system\n\toption hostname '"+hn+"'\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	writeHostname("before")
	b := &NativeBackend{uciDir: dir}
	if hn, err := b.Hostname(context.Background()); err != nil || hn != "before" {
		t.Fatalf("Hostname = %q, %v; want before", hn, err)
	}

	writeHostname("after") // external writer commits a change
	if hn, err := b.Hostname(context.Background()); err != nil || hn != "after" {
		t.Errorf("Hostname = %q, want after (stale cached read?)", hn)
	}
}

func fakeSystemInfo(m map[string]any, err error) ubusSystemInfo {
	return func(context.Context) (map[string]any, error) { return m, err }
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

	si, err := b.SystemInfo(context.Background())
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
	if _, err := b.SystemInfo(context.Background()); err == nil {
		t.Fatal("SystemInfo: want error, got nil")
	}
}

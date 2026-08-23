// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"testing"

	uci "github.com/digineo/go-uci"
)

func TestHostnameFromUCI(t *testing.T) {
	b := &NativeBackend{uci: uci.NewTree("testdata/config")}

	hn, err := b.Hostname(context.Background())
	if err != nil {
		t.Fatalf("Hostname: %v", err)
	}
	if hn != "verso-lab" {
		t.Errorf("Hostname = %q, want %q", hn, "verso-lab")
	}
}

func TestHostnameMissing(t *testing.T) {
	b := &NativeBackend{uci: uci.NewTree("testdata/empty")}
	if _, err := b.Hostname(context.Background()); err == nil {
		t.Fatal("Hostname: want error when config missing, got nil")
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

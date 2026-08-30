// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSamplerRetainsBoundedInterfaceHistory(t *testing.T) {
	root := t.TempDir()
	writeInterface(t, root, "eth0", "up", 100, 200, 10, 20)
	now := time.Unix(100, 0)
	sampler := newSampler(root, time.Hour, 2, func() time.Time { return now })
	sampler.sample()

	now = now.Add(time.Second)
	writeInterface(t, root, "eth0", "up", 200, 400, 20, 40)
	sampler.sample()
	now = now.Add(time.Second)
	writeInterface(t, root, "eth0", "up", 400, 800, 40, 80)
	sampler.sample()

	snapshot, err := sampler.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != "interfaces" || len(snapshot.Interfaces) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	history := snapshot.Interfaces[0].History
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].RxBPS != 800 || history[0].TxBPS != 1600 || history[1].RxBPS != 1600 || history[1].TxBPS != 3200 {
		t.Fatalf("history rates = %+v", history)
	}
}

func TestSamplerTreatsCounterResetAsZeroRate(t *testing.T) {
	root := t.TempDir()
	writeInterface(t, root, "eth0", "up", 100, 200, 10, 20)
	now := time.Unix(100, 0)
	sampler := newSampler(root, time.Hour, 60, func() time.Time { return now })
	sampler.sample()
	now = now.Add(time.Second)
	writeInterface(t, root, "eth0", "down", 1, 2, 1, 2)
	sampler.sample()

	snapshot, err := sampler.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	iface := snapshot.Interfaces[0]
	point := iface.History[len(iface.History)-1]
	if iface.Operstate != "down" || point.RxBPS != 0 || point.TxBPS != 0 {
		t.Fatalf("interface = %+v", iface)
	}
}

func TestSnapshotFreshness(t *testing.T) {
	now := time.Unix(10, 0)
	if !(Snapshot{TimestampMS: 9000}).Fresh(now, 2*time.Second) {
		t.Fatal("one-second-old snapshot should be fresh")
	}
	if (Snapshot{TimestampMS: 1000}).Fresh(now, 2*time.Second) {
		t.Fatal("nine-second-old snapshot should be stale")
	}
}

func TestSamplerClassifiesInterfaceTopology(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(filepath.Dir(root), "ieee80211", "mwiphy0"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"br-lan", "eth0", "eth0.10", "lo", "pppoe-wan", "sit0", "tailscale0", "wlan0", "uap0"} {
		writeInterface(t, root, name, "up", 1, 2, 3, 4)
	}
	for _, path := range []string{
		filepath.Join(root, "eth0", "device"),
		filepath.Join(root, "br-lan", "bridge"),
		filepath.Join(root, "br-lan", "brif", "eth0"),
		filepath.Join(root, "wlan0", "device"),
		filepath.Join(root, "wlan0", "wireless"),
		filepath.Join(root, "uap0", "device"),
		filepath.Join(root, "uap0", "phy80211"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sampler := newSampler(root, time.Hour, 60, func() time.Time { return time.Unix(100, 0) })
	sampler.sample()
	snapshot, err := sampler.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]struct {
		kind, parent string
		physical     bool
	}{
		"br-lan": {kind: "bridge"}, "eth0": {kind: "port", physical: true},
		"eth0.10": {kind: "vlan", parent: "eth0"}, "lo": {kind: "loopback"},
		"pppoe-wan": {kind: "pppoe"}, "sit0": {kind: "tunnel"},
		"tailscale0": {kind: "tunnel"},
		"wlan0":      {kind: "wifi"},
		"uap0":       {kind: "wifi"},
	}
	for name, want := range wants {
		got, ok := snapshot.Interface(name)
		if !ok || got.Kind != want.kind || got.Parent != want.parent || got.Physical != want.physical {
			t.Errorf("%s = %+v, want kind=%q parent=%q physical=%v", name, got, want.kind, want.parent, want.physical)
		}
	}
	bridge, _ := snapshot.Interface("br-lan")
	if len(bridge.Members) != 1 || bridge.Members[0] != "eth0" {
		t.Errorf("br-lan members = %v, want [eth0]", bridge.Members)
	}
	if len(snapshot.WirelessPHYs) != 1 || snapshot.WirelessPHYs[0] != "mwiphy0" {
		t.Errorf("wireless PHYs = %v, want [mwiphy0]", snapshot.WirelessPHYs)
	}
}

func writeInterface(t *testing.T, root, name, state string, rxBytes, txBytes, rxPackets, txPackets uint64) {
	t.Helper()
	stats := filepath.Join(root, name, "statistics")
	if err := os.MkdirAll(stats, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, name, "operstate"): state,
		filepath.Join(stats, "rx_bytes"):       uintString(rxBytes),
		filepath.Join(stats, "tx_bytes"):       uintString(txBytes),
		filepath.Join(stats, "rx_packets"):     uintString(rxPackets),
		filepath.Join(stats, "tx_packets"):     uintString(txPackets),
	}
	for path, value := range files {
		if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func uintString(value uint64) string {
	return strconv.FormatUint(value, 10)
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/telemetry"
)

func TestNetworkTopologySeparatesBridgeMembersFromTransport(t *testing.T) {
	for _, members := range [][]string{{"lan0"}, {}} {
		s := &Server{telemetry: fakeTelemetry{snapshot: telemetry.Snapshot{
			TimestampMS: uint64(time.Now().UnixMilli()),
			Interfaces: []telemetry.Interface{
				{Name: "br-lan", Kind: "bridge", Parent: "lan0", Members: members},
				{Name: "lan0", Kind: "virtual"},
				{Name: "br-lan.10", Kind: "vlan", Parent: "br-lan"},
			},
		}}}
		data := json.RawMessage(`{"devices":{"br-lan":{"parent":"wrong","bridge-members":["old-port"]}}}`)
		var state struct {
			Devices map[string]struct {
				Parent  string   `json:"parent"`
				Members []string `json:"bridge-members"`
			}
		}
		if err := json.Unmarshal(s.networkTopology(context.Background(), data), &state); err != nil {
			t.Fatal(err)
		}
		bridge := state.Devices["br-lan"]
		if bridge.Parent != "" || len(bridge.Members) != len(members) {
			t.Fatalf("bridge = %+v, want root with members %v", bridge, members)
		}
		if len(members) > 0 && bridge.Members[0] != "lan0" {
			t.Fatalf("members = %v", bridge.Members)
		}
		if state.Devices["lan0"].Parent != "" || state.Devices["br-lan.10"].Parent != "br-lan" {
			t.Fatalf("transport relationships = %+v", state.Devices)
		}
	}
}

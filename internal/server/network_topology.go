// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
)

// Add the same kernel topology the overview already samples, retaining netifd's
// addresses and device facts. A stale sample contributes no guessed state.
func (s *Server) networkTopology(ctx context.Context, data json.RawMessage) json.RawMessage {
	var state map[string]any
	if json.Unmarshal(data, &state) != nil {
		return data
	}
	snapshot, fresh := s.telemetrySnapshot(ctx)
	if !fresh {
		return data
	}
	devices, _ := state["devices"].(map[string]any)
	if devices == nil {
		devices = map[string]any{}
	}
	for _, iface := range snapshot.Interfaces {
		if iface.Name == "lo" {
			continue
		}
		device, _ := devices[iface.Name].(map[string]any)
		if device == nil {
			device = map[string]any{}
		}
		device["physical"] = iface.Physical
		device["kind"] = iface.Kind
		// lower_* on a bridge are its members, not its transport. A bridge
		// stays a root; the plugin groups its software members beneath it.
		if iface.Kind != "bridge" {
			device["parent"] = iface.Parent
		} else {
			delete(device, "parent")
		}
		if iface.Members != nil {
			device["bridge-members"] = iface.Members
		}
		device["operstate"] = iface.Operstate
		devices[iface.Name] = device
	}
	state["devices"] = devices
	out, err := json.Marshal(state)
	if err != nil {
		return data
	}
	return out
}

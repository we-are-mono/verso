// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

func TestFirewallActivityUsesIndependentCollector(t *testing.T) {
	backend := fwLogBackend()
	reads := 0
	backend.logReads = &reads
	backend.logErr = errStreamRead
	_, payload := openStream(t, newServer(t, backend), "firewall-log")
	if rows := decodeStreamRows(t, payload); len(rows) != 3 || reads != 0 {
		t.Fatalf("firewall activity must use NFLOG independently of logd: rows=%d logd reads=%d", len(rows), reads)
	}
}

func TestSystemLogsExcludePacketsButKeepDiagnostics(t *testing.T) {
	packetReads := 0
	backend := fakeBackend{logEntries: []openwrt.LogEntry{
		{ID: 0, Source: 0, Msg: "eth0: link down"},
		{ID: 1, Source: 1, Msg: "firewall: reload failed"},
		{ID: 2, Source: 0, Msg: fwLineRuleDrop},
	}}
	backend.firewallReads = &packetReads
	_, payload := openStream(t, newServer(t, backend), "system-log")
	var frame struct {
		Rows []systemLogEvent `json:"rows"`
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		t.Fatal(err)
	}
	if len(frame.Rows) != 2 || frame.Rows[0].Source != "kernel" || frame.Rows[1].Source != "firewall" {
		t.Fatalf("packet filter removed diagnostics or retained traffic: %+v", frame.Rows)
	}
	if packetReads != 0 {
		t.Fatal("default System Logs must not read the packet collector")
	}
}

func TestUnavailableCollectorIsNotReportedAsQuiet(t *testing.T) {
	backend := fwLogBackend()
	backend.firewallErr = errStreamRead
	backend.logEntries = []openwrt.LogEntry{{ID: 1, Msg: "link down"}}
	_, payload := openStream(t, newServer(t, backend), "firewall-log")
	var frame struct {
		Available bool
		Rows      []fwEvent
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Available || len(frame.Rows) != 0 {
		t.Fatalf("unavailable collector: %+v", frame)
	}
	_, payload = openStream(t, newServer(t, backend), "system-log?firewall=1")
	var combined struct {
		Rows        []systemLogEvent
		Unavailable bool `json:"firewall_unavailable"`
	}
	if err := json.Unmarshal([]byte(payload), &combined); err != nil {
		t.Fatal(err)
	}
	if !combined.Unavailable || len(combined.Rows) != 1 || combined.Rows[0].Message != "link down" {
		t.Fatalf("collector failure must retain diagnostics and discard partial packet reads: %+v", combined)
	}
}

func TestCollectorEpochResetsEvenWhenNewIDsHaveOvertakenTheOldCursor(t *testing.T) {
	reader := fwLogReader{backend: fwLogBackend(), sid: "sid", generation: "old", cursor: 1, started: true}
	rows := reader.poll(context.Background())
	if len(rows) != 3 || !reader.reset || reader.generation != "test" {
		t.Fatalf("new epoch must reset browser history regardless of counter size: %+v", reader)
	}
}

func TestSystemLogsCanIncludeSeparateFirewallBuffer(t *testing.T) {
	backend := fwLogBackend()
	backend.logEntries = []openwrt.LogEntry{{ID: 1, Time: 1788294050000, Source: 1, Msg: "firewall: reload complete"}}
	_, payload := openStream(t, newServer(t, backend), "system-log?firewall=1")
	var frame struct {
		Rows []systemLogEvent `json:"rows"`
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		t.Fatal(err)
	}
	if len(frame.Rows) != 4 || frame.Rows[0].ID != 1 || frame.Rows[1].ID != -4201 || frame.Rows[1].Source != "firewall" {
		t.Fatalf("combined view must merge independent IDs chronologically: %+v", frame.Rows)
	}
}

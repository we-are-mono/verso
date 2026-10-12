// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

func sections(t *testing.T, text string) map[string]map[string]any {
	t.Helper()
	var out map[string]map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestADirectEditReadsAsTheChangesUCIWouldList: each difference between two
// readings as `uci changes` would have put it, sections made and removed
// included, and a secret only as changed.
func TestADirectEditReadsAsTheChangesUCIWouldList(t *testing.T) {
	before := sections(t, `{
		"lan":  {".type":"interface",".index":0,"proto":"static","mtu":"1500","ports":["lan1","lan2"]},
		"wan":  {".type":"interface",".index":1,"password":{"secret":"aa"},"username":"u"},
		"wan6": {".type":"interface",".index":2,"proto":"dhcpv6"}
	}`)
	after := sections(t, `{
		"lan":   {".type":"interface",".index":0,"proto":"dhcp","ports":["lan1","lan3"],"dns":"1.1.1.1"},
		"wan":   {".type":"interface",".index":1,"password":{"secret":"bb"},"username":"u"},
		"guest": {".type":"interface",".index":2,"proto":"static"}
	}`)
	tuples, secret := diffConfig(before, after)
	want := [][]string{
		{"set", "lan", "dns", "1.1.1.1"},
		{"remove", "lan", "mtu"},
		{"list-add", "lan", "ports", "lan3"},
		{"list-del", "lan", "ports", "lan2"},
		{"set", "lan", "proto", "dhcp"},
		{"set", "wan", "password", ""},
		{"add", "guest", "interface"},
		{"set", "guest", "proto", "static"},
		{"remove", "wan6"},
	}
	if !reflect.DeepEqual(tuples, want) {
		t.Fatalf("tuples\n%v\nwant\n%v", tuples, want)
	}
	if !reflect.DeepEqual(secret, map[string]bool{"password": true}) {
		t.Errorf("secret = %v", secret)
	}
	if tuples, _ := diffConfig(before, before); len(tuples) != 0 {
		t.Errorf("a reading like the last differs by %v", tuples)
	}
}

// spoolServer is a server whose history.db and config history read a spool
// in a temporary directory, firewall written by a plugin.
func spoolServer(t *testing.T) (*Server, *history.Store, string) {
	t.Helper()
	s, store, _, _ := actsServer(t, nil)
	return s, store, t.TempDir()
}

func spool(t *testing.T, dir, name string, reading configReading) {
	t.Helper()
	data, _ := json.Marshal(reading)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func firewallReading(at int64, target string) configReading {
	return configReading{Time: at, Config: "firewall", Sections: map[string]map[string]any{
		"cfg0792bd": {".type": "rule", ".index": 0, "name": "Allow-Ping", "target": target},
	}}
}

func entries(t *testing.T, store *history.Store) []history.Change {
	t.Helper()
	all, err := store.Changes(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// TestAConfigWrittenDirectlyIsAnEntryByNoOne: the first reading is where
// history starts; one that differs from it is an entry of its own, by no
// one, at the time it was written, with what each change replaced.
func TestAConfigWrittenDirectlyIsAnEntryByNoOne(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)
	if got := entries(t, store); len(got) != 0 {
		t.Fatalf("the first reading made entries: %+v", got)
	}
	spool(t, dir, "2-firewall.json", firewallReading(2000, "DROP"))
	s.takeSpool(t.Context(), store, dir)

	got := entries(t, store)
	if len(got) != 1 {
		t.Fatalf("entries = %+v, want one", got)
	}
	c := got[0]
	if c.User != "" || c.Address != "" || !c.Time.Equal(time.Unix(2000, 0)) || !c.OldRead || c.Status != history.ChangeApplied {
		t.Errorf("entry = %+v", c)
	}
	op := c.Items[0].Ops[0]
	if c.Items[0].Page != "Demo Plugin" || op.Op != "set" || op.Option != "target" || *op.Value != "DROP" || string(op.Old) != `"ACCEPT"` {
		t.Errorf("item %+v op %+v", c.Items[0], op)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("readings left in the spool: %v", left)
	}
}

// TestAReadingVersoWroteItselfIsNoEntry: the helper's own write, and an
// apply's, keep the reading without an entry.
func TestAReadingVersoWroteItselfIsNoEntry(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)

	own := firewallReading(1010, "DROP")
	own.Own = true
	spool(t, dir, "2-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)

	s.expected.expect([]string{"firewall"}, time.Unix(1020, 0), time.Unix(1030, 0))
	spool(t, dir, "3-firewall.json", firewallReading(1025, "REJECT"))
	s.takeSpool(t.Context(), store, dir)

	if got := entries(t, store); len(got) != 0 {
		t.Fatalf("Verso's own writes made entries: %+v", got)
	}
	state, _, _ := store.ConfigState(t.Context(), "firewall")
	if !json.Valid(state) || !reflect.DeepEqual(sections(t, string(state))["cfg0792bd"]["target"], "REJECT") {
		t.Errorf("the config is kept as %s, want as last read", state)
	}
}

// TestReadingsWaitForAnApplysWindow: while an apply may still be rolled
// back, nothing is taken.
func TestReadingsWaitForAnApplysWindow(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.applies.by = map[string]pendingApply{"test-sid": {seq: 1}}
	s.takeSpool(t.Context(), store, dir)
	if left, _ := os.ReadDir(dir); len(left) != 1 {
		t.Fatalf("the reading was taken during the window: %v", left)
	}
}

// TestAConfigNoPluginWritesIsNotKept: config history is the configs Verso
// manages.
func TestAConfigNoPluginWritesIsNotKept(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-dropbear.json", configReading{Time: 1, Config: "dropbear", Sections: map[string]map[string]any{}})
	s.takeSpool(t.Context(), store, dir)
	if _, ok, _ := store.ConfigState(t.Context(), "dropbear"); ok {
		t.Error("an undeclared config was kept")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("readings left in the spool: %v", left)
	}
}

// TestAnApplyExpectsItsOwnWrites: once recorded, an apply's configs read
// from its start until a little after as its own.
func TestAnApplyExpectsItsOwnWrites(t *testing.T) {
	s, log, window := changeServer(t, map[string][][]string{"system": {{"set", "main", "hostname", "edge"}}})
	before := time.Now()
	applyConfirmed(t, s, window)
	if len(log.changes) != 1 {
		t.Fatal("no apply recorded")
	}
	if !s.expected.holds("system", before) || !s.expected.holds("system", time.Now()) {
		t.Error("the apply's own write to system is not expected")
	}
	if s.expected.holds("network", before) || s.expected.holds("system", time.Now().Add(time.Minute)) {
		t.Error("a write the apply did not make is expected")
	}
}

// TestHistoryKeepsAsManyAsItIsTold: past the limit the oldest entry leaves,
// and the starting point moves on by it.
func TestHistoryKeepsAsManyAsItIsTold(t *testing.T) {
	s, store, _, window := actsServer(t, map[string][][]string{"firewall": {{"set", "cfg0792bd", "target", "DROP"}}})
	s.SetChangesKept(1)
	if err := store.StartConfig(t.Context(), "firewall", json.RawMessage(`{"cfg0792bd":{".type":"rule","target":"ACCEPT"}}`), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	applyConfirmed(t, s, window)
	applyConfirmed(t, s, window)
	if got := entries(t, store); len(got) != 1 {
		t.Fatalf("kept %d entries, want 1", len(got))
	}
	start, _ := store.StartingPoint(t.Context())
	if !strings.Contains(string(start.Configs["firewall"]), `"target":"DROP"`) {
		t.Errorf("the starting point did not move on: %s", start.Configs["firewall"])
	}
}

// TestAFirstReadingIsTheStartingPoint.
func TestAFirstReadingIsTheStartingPoint(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)
	spool(t, dir, "2-firewall.json", firewallReading(2000, "DROP"))
	s.takeSpool(t.Context(), store, dir)
	start, _ := store.StartingPoint(t.Context())
	if !strings.Contains(string(start.Configs["firewall"]), `"target":"ACCEPT"`) {
		t.Errorf("starting point = %s, want the first reading", start.Configs["firewall"])
	}
}

// TestTheLimitIsOneAtLeast: a limit below one keeps the default.
func TestTheLimitIsOneAtLeast(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for _, n := range []int{0, -5} {
		s.SetChangesKept(n)
		if s.changesKept != changesKeptDefault {
			t.Errorf("limit %d kept %d, want %d", n, s.changesKept, changesKeptDefault)
		}
	}
}

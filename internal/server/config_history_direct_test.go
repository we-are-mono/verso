// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/alerts"
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

// TestASecretNotKeptIsNoChange: a secret's value the history does not keep,
// as a scrub leaves one, is nothing to compare a fingerprint with, or a
// fingerprint with it: either reads as no change. Set where it was not, or
// removed, it is a change still.
func TestASecretNotKeptIsNoChange(t *testing.T) {
	notKept := `{".type":"alerts",".index":0,"url":{"secret":true}}`
	fingerprint := `{".type":"alerts",".index":0,"url":{"secret":"aa"}}`
	none := `{".type":"alerts",".index":0}`
	for _, c := range []struct {
		name, before, after string
		want                [][]string
	}{
		{"not kept, then a fingerprint", notKept, fingerprint, nil},
		{"a fingerprint, then not kept", fingerprint, notKept, nil},
		{"not kept, then not kept", notKept, notKept, nil},
		{"not kept, then removed", notKept, none, [][]string{{"remove", "alerts", "url"}}},
		{"none, then a fingerprint", none, fingerprint, [][]string{{"set", "alerts", "url", ""}}},
	} {
		before := sections(t, `{"alerts":`+c.before+`}`)
		after := sections(t, `{"alerts":`+c.after+`}`)
		if tuples, _ := diffConfig(before, after); !reflect.DeepEqual(tuples, c.want) {
			t.Errorf("%s: tuples %v, want %v", c.name, tuples, c.want)
		}
	}
}

// TestAnUpgradeListingASecretRecordsNoEdit: a shell whose list left out the
// alert address kept it in plaintext as the config last read; the scrub as
// the history opens leaves it not kept, and the helper's first reading, its
// fingerprint for the same address, is no edit made directly.
func TestAnUpgradeListingASecretRecordsNoEdit(t *testing.T) {
	dir := t.TempDir()
	written, err := history.Open(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	alertsSection := func(url any) map[string]map[string]any {
		return map[string]map[string]any{"alerts": {".type": "alerts", ".name": "alerts", ".anonymous": false,
			".index": float64(0), "url": url, "kind": []any{"boot"}}}
	}
	held, _ := json.Marshal(alertsSection("https://ntfy.sh/topic"))
	if err := errors.Join(written.SetConfigState(t.Context(), "verso", held, time.Unix(60, 0)),
		written.StartConfig(t.Context(), "verso", held, time.Unix(0, 0)), written.Close()); err != nil {
		t.Fatal(err)
	}

	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)
	store, onDisk := s.openHistory(dir, alerts.Config{})
	if store == nil || !onDisk {
		t.Fatalf("history not opened on disk: %v, %v", store, onDisk)
	}
	t.Cleanup(func() { _ = store.Close() })
	reading := configReading{Time: 120, Config: "verso",
		Sections: alertsSection(map[string]any{"secret": "0123456789abcdef0123456789abcdef"})}
	if err := s.takeReading(t.Context(), store, reading); err != nil {
		t.Fatal(err)
	}
	if got := entries(t, store); len(got) != 0 {
		t.Errorf("the helper's first reading made entries: %+v", got)
	}
	state, _, err := store.ConfigState(t.Context(), "verso")
	if err != nil || !strings.Contains(string(state), "0123456789abcdef") {
		t.Errorf("the reading is not kept as the config last read: %s, %v", state, err)
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

// TestTheRoutersOwnCommitIsAnEntryWithItsCause: a commit the router made
// itself is an entry by no operator, with what caused it and whose words the
// cause is, each change in the owning plugin's words; the same reading again
// is no entry (ADR-021).
func TestTheRoutersOwnCommitIsAnEntryWithItsCause(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)

	own := firewallReading(1010, "DROP")
	own.Own, own.Cause, own.Plugin = true, "A certificate was installed by hand.", "acme"
	spool(t, dir, "2-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)
	own.Time = 1015
	spool(t, dir, "3-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)

	got := entries(t, store)
	if len(got) != 1 {
		t.Fatalf("entries = %+v, want one", got)
	}
	c := got[0]
	if c.User != "" || c.Address != "" || c.Cause != "A certificate was installed by hand." || c.CausePlugin != "acme" ||
		!c.OldRead || !c.Time.Equal(time.Unix(1010, 0)) || c.Status != history.ChangeApplied {
		t.Errorf("entry = %+v", c)
	}
	op := c.Items[0].Ops[0]
	if c.Items[0].Page != "Demo Plugin" || op.Option != "target" || *op.Value != "DROP" || string(op.Old) != `"ACCEPT"` {
		t.Errorf("item %+v op %+v", c.Items[0], op)
	}
}

// TestACommitMadeForAnOperatorIsTheirs: a commit the router made as part of
// an operator's act names that operator and where they acted from, by the
// handle of their session; a handle no session holds, or none, is the
// router's; a reading not the router's own is still edited directly
// (ADR-021).
func TestACommitMadeForAnOperatorIsTheirs(t *testing.T) {
	s, store, dir := spoolServer(t)
	s.sessions.CreateWithMetadata("rpcd-sid-of-root", "root", "192.0.2.10", "test")
	digest := sha256.Sum256([]byte("rpcd-sid-of-root"))
	handle := hex.EncodeToString(digest[:])

	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)
	steps := []struct {
		at     int64
		target string
		own    bool
		by     string
	}{
		{1010, "DROP", true, handle},
		{1020, "REJECT", true, strings.Repeat("0", 64)},
		{1030, "ACCEPT", true, ""},
		{1040, "DROP", false, handle},
	}
	for i, step := range steps {
		r := firewallReading(step.at, step.target)
		r.Own, r.Cause, r.By = step.own, "The theme was changed.", step.by
		spool(t, dir, fmt.Sprintf("%d-firewall.json", i+2), r)
		s.takeSpool(t.Context(), store, dir)
	}

	got := entries(t, store)
	if len(got) != 4 {
		t.Fatalf("entries = %+v, want four", got)
	}
	// Newest first.
	byOperator, unknown, none, direct := got[3], got[2], got[1], got[0]
	if byOperator.User != "root" || byOperator.Address != "192.0.2.10" || !byOperator.Own || byOperator.Cause != "The theme was changed." {
		t.Errorf("the operator's commit = %+v", byOperator)
	}
	for name, c := range map[string]history.Change{"an unknown handle": unknown, "no handle": none} {
		if c.User != "" || c.Address != "" || !c.Own {
			t.Errorf("%s = %+v, want the router's", name, c)
		}
	}
	if direct.User != "" || direct.Own {
		t.Errorf("a reading not the router's own = %+v", direct)
	}
}

// TestACausesArgumentsAreKept: a package operation's commit keeps its
// cause's arguments, the package it names.
func TestACausesArgumentsAreKept(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)
	r := firewallReading(1010, "DROP")
	r.Own, r.Cause, r.Args = true, "Changed by %s as it was installed.", []string{"https-dns-proxy"}
	spool(t, dir, "2-firewall.json", r)
	s.takeSpool(t.Context(), store, dir)
	got := entries(t, store)
	if len(got) != 1 || !reflect.DeepEqual(got[0].CauseArgs, []string{"https-dns-proxy"}) || got[0].Cause != r.Cause {
		t.Fatalf("entries = %+v", got)
	}
}

// TestAReadingInAnApplysWindowIsTheApplys: a reading of a config an apply
// wrote, within its window, is that apply's, its own mark or not.
func TestAReadingInAnApplysWindowIsTheApplys(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)

	s.expected.expect([]string{"firewall"}, time.Unix(1020, 0), time.Unix(1030, 0))
	spool(t, dir, "2-firewall.json", firewallReading(1025, "REJECT"))
	s.takeSpool(t.Context(), store, dir)
	own := firewallReading(1028, "DROP")
	own.Own, own.Cause = true, "The theme was changed."
	spool(t, dir, "3-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)

	if got := entries(t, store); len(got) != 0 {
		t.Fatalf("readings in an apply's window made entries: %+v", got)
	}
	state, _, _ := store.ConfigState(t.Context(), "firewall")
	if !json.Valid(state) || !reflect.DeepEqual(sections(t, string(state))["cfg0792bd"]["target"], "DROP") {
		t.Errorf("the config is kept as %s, want as last read", state)
	}
}

// TestAnOlderPluginsMarkIsTheRoutersWithNoCause: a root half built before
// causes marks its commit with nothing in the mark; the entry is still the
// router's, never edited directly, with no cause.
func TestAnOlderPluginsMarkIsTheRoutersWithNoCause(t *testing.T) {
	s, store, dir := spoolServer(t)
	spool(t, dir, "1-firewall.json", firewallReading(1000, "ACCEPT"))
	s.takeSpool(t.Context(), store, dir)
	own := firewallReading(1010, "DROP")
	own.Own = true
	spool(t, dir, "2-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)

	got := entries(t, store)
	if len(got) != 1 || got[0].User != "" || got[0].Cause != "" || got[0].CausePlugin != "" || !got[0].Own {
		t.Fatalf("entries = %+v, want one of the router's with no cause", got)
	}
}

// TestTheRoutersOwnCommitKeepsNoSecret: a secret the router commits reads as
// changed, holding neither value.
func TestTheRoutersOwnCommitKeepsNoSecret(t *testing.T) {
	s, store, dir := spoolServer(t)
	withKey := func(at int64, fingerprint string) configReading {
		r := firewallReading(at, "ACCEPT")
		r.Sections["cfg0792bd"]["key"] = map[string]any{"secret": fingerprint}
		return r
	}
	spool(t, dir, "1-firewall.json", withKey(1000, "aa"))
	s.takeSpool(t.Context(), store, dir)
	own := withKey(1010, "bb")
	own.Own, own.Cause = true, "The web interface's ports were changed."
	spool(t, dir, "2-firewall.json", own)
	s.takeSpool(t.Context(), store, dir)

	got := entries(t, store)
	if len(got) != 1 {
		t.Fatalf("entries = %+v, want one", got)
	}
	op := got[0].Items[0].Ops[0]
	if op.Option != "key" || (op.Value != nil && *op.Value != "") || len(op.Old) != 0 {
		t.Errorf("a secret kept a value: %+v", op)
	}
	blob, _ := json.Marshal(got)
	if strings.Contains(string(blob), `"aa"`) || strings.Contains(string(blob), `"bb"`) {
		t.Errorf("a fingerprint was kept: %s", blob)
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

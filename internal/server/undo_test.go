// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

// stageFake is a stage that keeps its journal as rpcd does: each write is an
// entry, a list written whole is its option removed and each value added, a
// config's uci entries and its staged files are let go apart. failOn names
// one write ("set lan leasetime") that fails, once.
type stageFake struct {
	fakeBackend
	j *stageJournal
}

type stageJournal struct {
	uci, files map[string][][]string
	next       int
	failOn     string
	denied     bool     // every write refused, as for a session rpcd no longer knows
	unstaged   []string // "uci:config" or "files:config", in order
	orders     []string
}

func newStageFake(uci, files map[string][][]string) stageFake {
	f := stageFake{fakeBackend: fakeBackend{access: true}, j: &stageJournal{uci: map[string][][]string{}, files: map[string][][]string{}}}
	for c, l := range uci {
		f.j.uci[c] = slices.Clone(l)
	}
	for c, l := range files {
		f.j.files[c] = slices.Clone(l)
	}
	return f
}

func (f stageFake) fail(entry string) error {
	if f.j.failOn != "" && f.j.failOn == entry {
		f.j.failOn = ""
		return errors.New("refused")
	}
	return nil
}

func (f stageFake) UCIChanges(context.Context, string) (map[string][][]string, error) {
	out := map[string][][]string{}
	for c, l := range f.j.uci {
		out[c] = slices.Clone(l)
	}
	for c, l := range f.j.files {
		out[c] = append(out[c], l...)
	}
	return out, nil
}

func (f stageFake) UCISet(_ context.Context, _, config, section string, values map[string]any) error {
	if f.j.denied {
		return errors.New("access denied")
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := f.fail("set " + section + " " + k); err != nil {
			return err
		}
		switch v := values[k].(type) {
		case string:
			f.j.uci[config] = append(f.j.uci[config], []string{"set", section, k, v})
		case []string:
			f.j.uci[config] = append(f.j.uci[config], []string{"remove", section, k})
			for _, item := range v {
				f.j.uci[config] = append(f.j.uci[config], []string{"list-add", section, k, item})
			}
		default:
			return fmt.Errorf("unexpected value %T", v)
		}
	}
	return nil
}

func (f stageFake) UCIAdd(_ context.Context, _, config, secType, name string) (string, error) {
	if name == "" {
		f.j.next++
		name = fmt.Sprintf("cfg%06x", 0xa00000+f.j.next)
	}
	f.j.uci[config] = append(f.j.uci[config], []string{"add", name, secType})
	return name, nil
}

func (f stageFake) UCIDelete(_ context.Context, _, config, section, option string) error {
	if option == "" {
		f.j.uci[config] = append(f.j.uci[config], []string{"remove", section})
		return nil
	}
	f.j.uci[config] = append(f.j.uci[config], []string{"remove", section, option})
	return nil
}

func (f stageFake) UCIOrder(_ context.Context, _, config string, sections []string) error {
	f.j.orders = append(f.j.orders, fmt.Sprintf("%s: %v", config, sections))
	return nil
}

// UCIApply commits the stage, so the journal empties, as rpcd's does; denied
// stands for a session rpcd no longer knows.
func (f stageFake) UCIApply(context.Context, string, int) error {
	f.j.uci, f.j.files = map[string][][]string{}, map[string][][]string{}
	return nil
}

func (f stageFake) UCIUnstage(_ context.Context, _, config string) error {
	f.j.unstaged = append(f.j.unstaged, "uci:"+config)
	delete(f.j.uci, config)
	return nil
}

func (f stageFake) UnstageFiles(_ context.Context, _, config string) error {
	f.j.unstaged = append(f.j.unstaged, "files:"+config)
	delete(f.j.files, config)
	return nil
}

func dhcpManifest() plugin.Manifest {
	m := demoManifest()
	m.ID, m.Name = "dnsdhcp", "DNS & DHCP"
	m.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "dhcp", Function: "write"}, {Scope: "uci", Object: "firewall", Function: "write"}}}
	return m
}

// rowIDs is each row's id, keyed by its first change's raw line.
func rowIDs(t *testing.T, s *Server) map[string]string {
	t.Helper()
	identity := func(s string) string { return s }
	view := s.staged(context.Background(), "test-sid", identity, func(string) func(string) string { return identity })
	ids := map[string]string{}
	for _, g := range view.Groups {
		for _, item := range g.Items {
			ids[item.changes[0].raw] = item.ID
		}
	}
	return ids
}

// TestUndoOneOfTwoChangesInAConfig: the config is let go and staged again
// without the undone change; the other keeps its value and when it was
// staged.
func TestUndoOneOfTwoChangesInAConfig(t *testing.T) {
	f := newStageFake(map[string][][]string{"dhcp": {
		{"set", "lan", "leasetime", "6h"},
		{"set", "cfg01", "cachesize", "151"},
	}}, nil)
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	staged := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	s.authors.note("test-sid", optionAddress("dhcp", "cfg01", "cachesize"), stageAuthor{Plugin: "dnsdhcp", Page: "DHCP", At: staged})

	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	if _, err := s.undoRows(context.Background(), "test-sid", []string{id}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if want := [][]string{{"set", "cfg01", "cachesize", "151"}}; !reflect.DeepEqual(f.j.uci["dhcp"], want) {
		t.Errorf("dhcp's stage = %v, want %v", f.j.uci["dhcp"], want)
	}
	if who, ok := s.authors.of("test-sid", optionAddress("dhcp", "cfg01", "cachesize")); !ok || !who.At.Equal(staged) {
		t.Errorf("the kept change's record = %+v, want its own time kept", who)
	}
}

// TestUndoKeepsAnAddedSectionWhole: a section the stage made is made again
// under the name rpcd gives it now, and every write to it follows the name.
func TestUndoKeepsAnAddedSectionWhole(t *testing.T) {
	f := newStageFake(map[string][][]string{"firewall": {
		{"add", "cfg0992bd", "rule"},
		{"set", "cfg0992bd", "name", "Block-Telnet"},
		{"set", "cfg0992bd", "dest_port", "23"},
		{"set", "wan", "input", "DROP"},
	}}, nil)
	s := newServerWith(t, f, &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Added the rule Block-Telnet.", Covers: []int{0, 1, 2}},
		{Plain: "WAN input drops.", Covers: []int{3}},
	}}, []plugin.Manifest{dhcpManifest()})
	s.authors.note("test-sid", sectionAddress("firewall", "cfg0992bd"), stageAuthor{Plugin: "dnsdhcp", Page: "Rules"})

	id := rowIDs(t, s)["firewall: wan.input = DROP"]
	if _, err := s.undoRows(context.Background(), "test-sid", []string{id}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	got := f.j.uci["firewall"]
	if len(got) != 3 || got[0][0] != "add" || got[0][2] != "rule" {
		t.Fatalf("firewall's stage = %v, want the rule made again", got)
	}
	name := got[0][1]
	if want := [][]string{{"add", name, "rule"}, {"set", name, "name", "Block-Telnet"}, {"set", name, "dest_port", "23"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("firewall's stage = %v, want %v", got, want)
	}
	if _, ok := s.authors.of("test-sid", sectionAddress("firewall", name)); !ok {
		t.Error("the rule's record does not follow its new name")
	}
}

// TestUndoReplaysListsAndOrder: a list kept is written whole again, as rpcd
// journals it; a stage that moved sections keeps the order it staged.
func TestUndoReplaysListsAndOrder(t *testing.T) {
	f := newStageFake(map[string][][]string{"dhcp": {
		{"remove", "lan", "dhcp_option"},
		{"list-add", "lan", "dhcp_option", "6,1.1.1.1"},
		{"list-add", "lan", "dhcp_option", "15,lan"},
		{"order", "host_b", "0"},
		{"set", "lan", "leasetime", "6h"},
	}}, nil)
	f.uci = map[string]map[string]any{"dhcp": {
		"host_b": map[string]any{".index": int64(0)},
		"host_a": map[string]any{".index": int64(1)},
		"lan":    map[string]any{".index": int64(2)},
	}}
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})

	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	if _, err := s.undoRows(context.Background(), "test-sid", []string{id}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	want := [][]string{
		{"remove", "lan", "dhcp_option"},
		{"list-add", "lan", "dhcp_option", "6,1.1.1.1"},
		{"list-add", "lan", "dhcp_option", "15,lan"},
	}
	if !reflect.DeepEqual(f.j.uci["dhcp"], want) {
		t.Errorf("dhcp's stage = %v, want %v", f.j.uci["dhcp"], want)
	}
	if want := []string{"dhcp: [host_b host_a lan]"}; !reflect.DeepEqual(f.j.orders, want) {
		t.Errorf("orders = %v, want the staged order kept, %v", f.j.orders, want)
	}
}

// TestUndoLeavesStagedFilesAlone: undoing a setting in a config whose daemon
// reads staged files lets go of its uci changes alone; undoing the file lets
// go of the files alone.
func TestUndoLeavesStagedFilesAlone(t *testing.T) {
	f := newStageFake(
		map[string][][]string{"dhcp": {{"set", "lan", "leasetime", "6h"}}},
		map[string][][]string{"dhcp": {{"file", "/etc/dnsmasq.d/extra.conf", "address=/x/1.2.3.4"}}},
	)
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	ids := rowIDs(t, s)

	if _, err := s.undoRows(context.Background(), "test-sid", []string{ids["dhcp: lan.leasetime = 6h"]}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if want := []string{"uci:dhcp"}; !reflect.DeepEqual(f.j.unstaged, want) || len(f.j.files["dhcp"]) != 1 {
		t.Errorf("unstaged %v with files %v, want dhcp's uci alone let go and its file kept", f.j.unstaged, f.j.files["dhcp"])
	}

	f.j.unstaged = nil
	if _, err := s.undoRows(context.Background(), "test-sid", []string{ids["/etc/dnsmasq.d/extra.conf"]}); err != nil {
		t.Fatalf("undo of the file: %v", err)
	}
	if want := []string{"files:dhcp"}; !reflect.DeepEqual(f.j.unstaged, want) {
		t.Errorf("unstaged %v, want dhcp's files alone let go", f.j.unstaged)
	}
}

// TestUndoThatFailsRestoresTheStage: a write refused part way through the
// replay leaves the stage as it was before the undo, and says it failed.
func TestUndoThatFailsRestoresTheStage(t *testing.T) {
	before := [][]string{
		{"set", "lan", "leasetime", "6h"},
		{"set", "lan", "start", "50"},
		{"set", "cfg01", "cachesize", "151"},
	}
	f := newStageFake(map[string][][]string{"dhcp": before}, nil)
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	f.j.failOn = "set cfg01 cachesize"

	if _, err := s.undoRows(context.Background(), "test-sid", []string{id}); err == nil {
		t.Fatal("an undo whose replay failed says it succeeded")
	}
	if !reflect.DeepEqual(f.j.uci["dhcp"], before) {
		t.Errorf("dhcp's stage = %v, want it as it was, %v", f.j.uci["dhcp"], before)
	}
}

// TestUndoOfAReorderThatFailsRestoresTheOrder: undoing the section reorder
// itself, with a replay that fails, stages the config again in the order the
// stage held before the undo.
func TestUndoOfAReorderThatFailsRestoresTheOrder(t *testing.T) {
	f := newStageFake(map[string][][]string{"dhcp": {
		{"order", "host_b", "0"},
		{"set", "lan", "leasetime", "6h"},
	}}, nil)
	f.uci = map[string]map[string]any{"dhcp": {
		"host_b": map[string]any{".index": int64(0)},
		"host_a": map[string]any{".index": int64(1)},
		"lan":    map[string]any{".index": int64(2)},
	}}
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: order host_b 0"]
	f.j.failOn = "set lan leasetime"

	if _, err := s.undoRows(context.Background(), "test-sid", []string{id}); err == nil {
		t.Fatal("an undo whose replay failed says it succeeded")
	}
	if want := []string{"dhcp: [host_b host_a lan]"}; !reflect.DeepEqual(f.j.orders, want) {
		t.Errorf("orders = %v, want the staged order restored, %v", f.j.orders, want)
	}
}

// TestPutBackStagesTheUndoneAgain: what an undo took out is staged again as
// it was, with the time it was first staged.
func TestPutBackStagesTheUndoneAgain(t *testing.T) {
	f := newStageFake(map[string][][]string{"dhcp": {
		{"set", "lan", "leasetime", "6h"},
		{"set", "cfg01", "cachesize", "151"},
	}}, nil)
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	staged := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	s.authors.note("test-sid", optionAddress("dhcp", "lan", "leasetime"), stageAuthor{Plugin: "dnsdhcp", Page: "DHCP", At: staged})

	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	undone, err := s.undoRows(context.Background(), "test-sid", []string{id})
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if err := s.putBack(context.Background(), "test-sid", undone); err != nil {
		t.Fatalf("put back: %v", err)
	}
	want := [][]string{{"set", "cfg01", "cachesize", "151"}, {"set", "lan", "leasetime", "6h"}}
	if !reflect.DeepEqual(f.j.uci["dhcp"], want) {
		t.Errorf("dhcp's stage = %v, want %v", f.j.uci["dhcp"], want)
	}
	if who, ok := s.authors.of("test-sid", optionAddress("dhcp", "lan", "leasetime")); !ok || !who.At.Equal(staged) {
		t.Errorf("the change put back has record %+v, want its first time", who)
	}
}

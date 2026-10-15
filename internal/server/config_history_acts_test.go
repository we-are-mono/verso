// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// entry is an applied change of one row on the Interfaces page, its old
// values read.
func entry(id int64, ops ...history.ChangeOp) history.Change {
	return history.Change{ID: id, Time: time.Unix(id*60, 0), Status: history.ChangeApplied, OldRead: true,
		Items: []history.ChangeItem{{Page: "Interfaces", Plugin: "interfaces", Verb: "changed", Ops: ops}}}
}

func old(v string) json.RawMessage { return json.RawMessage(v) }

func set(section, option, value, was string) history.ChangeOp {
	op := history.ChangeOp{Config: "network", Section: section, Op: "set", Option: option, Value: text(value)}
	if was != "" {
		op.Old = old(was)
	}
	return op
}

// stepsOf is a plan's writes, without their authors.
func stepsOf(p actPlan) []plugin.CommitOp {
	var ops []plugin.CommitOp
	for _, s := range p.Steps {
		ops = append(ops, s.op)
	}
	return ops
}

// TestUndoPutsBackWhatEachChangeReplaced: a value back, a value that was not
// there cleared, a list whole, an option removed set again.
func TestUndoPutsBackWhatEachChangeReplaced(t *testing.T) {
	plan := reverseOf([]history.Change{entry(1,
		set("lan", "ipaddr", "192.168.2.1", `"192.168.1.1"`),
		set("lan", "mtu", "1400", ""),
		history.ChangeOp{Config: "network", Section: "lan", Op: "list-add", Option: "ports", Value: text("lan3"), Old: old(`["lan1","lan2"]`)},
		history.ChangeOp{Config: "network", Section: "lan", Op: "remove-option", Option: "ip6assign", Old: old(`"60"`)},
	)})
	want := []plugin.CommitOp{{Config: "network", Section: "lan", Values: map[string]any{
		"ipaddr": "192.168.1.1", "mtu": nil, "ports": []string{"lan1", "lan2"}, "ip6assign": "60",
	}}}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
	if plan.Steps[0].plugin != "interfaces" || plan.Steps[0].page != "Interfaces" {
		t.Errorf("staged under %q %q, want the row's plugin and page", plan.Steps[0].plugin, plan.Steps[0].page)
	}
	wantRows := []planRow{
		{"network.lan.ipaddr", "192.168.2.1", "192.168.1.1"},
		{"network.lan.mtu", "1400", "—"},
		{"network.lan.ports", "lan1 lan2 lan3", "lan1 lan2"},
		{"network.lan.ip6assign", "—", "60"},
	}
	if !reflect.DeepEqual(plan.Rows, wantRows) {
		t.Errorf("rows\n%v\nwant\n%v", plan.Rows, wantRows)
	}
}

// TestUndoOfSectionsRemovesWhatWasMadeAndMakesWhatWasRemoved: a section made
// goes, whatever was set on it; one removed comes back with what it held,
// named as it was, or anonymous as it was.
func TestUndoOfSectionsRemovesWhatWasMadeAndMakesWhatWasRemoved(t *testing.T) {
	plan := reverseOf([]history.Change{entry(1,
		history.ChangeOp{Config: "network", Section: "guest", Op: "add-section", Option: "interface"},
		set("guest", "proto", "static", ""),
		history.ChangeOp{Config: "network", Section: "wan6", Op: "remove-section", Old: old(`{".type":"interface",".name":"wan6",".anonymous":false,"proto":"dhcpv6","dns":["1.1.1.1"]}`)},
		history.ChangeOp{Config: "network", Section: "cfg0a1b2c", Op: "remove-section", Old: old(`{".type":"route",".anonymous":true,"target":"10.0.0.0/8"}`)},
	)})
	want := []plugin.CommitOp{
		{Config: "network", Section: "guest", Delete: true},
		{Config: "network", Section: "wan6", Type: "interface", Values: map[string]any{"proto": "dhcpv6", "dns": []string{"1.1.1.1"}}},
		{Config: "network", Type: "route", Values: map[string]any{"target": "10.0.0.0/8"}},
	}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
}

// TestUndoLeavesSecretsAndFilesAsTheyAre: neither has a value to put back.
func TestUndoLeavesSecretsAndFilesAsTheyAre(t *testing.T) {
	plan := reverseOf([]history.Change{entry(1,
		history.ChangeOp{Config: "network", Section: "wan", Op: "set", Option: "password", Secret: true},
		set("wan", "username", "new", `"old"`),
		history.ChangeOp{Config: "dhcp", Section: "dnsmasq.conf", Op: "file", Value: text("x")},
	)})
	if got := stepsOf(plan); !reflect.DeepEqual(got, []plugin.CommitOp{{Config: "network", Section: "wan", Values: map[string]any{"username": "old"}}}) {
		t.Fatalf("steps = %#v", got)
	}
	if !reflect.DeepEqual(plan.Secrets, []string{"network.wan.password"}) || !reflect.DeepEqual(plan.Files, []string{"dhcp.dnsmasq.conf"}) {
		t.Errorf("secrets %v, files %v", plan.Secrets, plan.Files)
	}
}

// TestNothingIsUndoneToValuesNeverRead: an entry recorded without what it
// replaced has nothing to put back.
func TestNothingIsUndoneToValuesNeverRead(t *testing.T) {
	c := entry(1, set("lan", "mtu", "1400", ""))
	c.OldRead = false
	if plan := reverseOf([]history.Change{c}); !plan.Unknown {
		t.Fatalf("plan = %+v, want it unknown", plan)
	}
	// A section made has nothing to read: it goes.
	c = entry(1, history.ChangeOp{Config: "network", Section: "guest", Op: "add-section", Option: "interface"})
	c.OldRead = false
	if plan := reverseOf([]history.Change{c}); plan.Unknown || len(plan.Steps) != 1 {
		t.Fatalf("plan = %+v, want the section removed", plan)
	}
}

// TestReturnPutsBackWhatWasThereBeforeTheFirst: over several entries each
// option returns to what the first to touch it replaced, and one changed and
// changed back needs nothing.
func TestReturnPutsBackWhatWasThereBeforeTheFirst(t *testing.T) {
	plan := reverseOf([]history.Change{
		entry(2, set("lan", "mtu", "1400", `"1500"`), set("lan", "proto", "dhcp", `"static"`)),
		entry(3, set("lan", "mtu", "9000", `"1400"`), set("lan", "proto", "static", `"dhcp"`)),
	})
	want := []plugin.CommitOp{{Config: "network", Section: "lan", Values: map[string]any{"mtu": "1500"}}}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
	if !reflect.DeepEqual(plan.Rows, []planRow{{"network.lan.mtu", "9000", "1500"}}) {
		t.Errorf("rows = %v", plan.Rows)
	}
}

// TestStageAgainWritesWhatTheEntryWrote: values as they were set, a list as
// it ended, a section made again (anonymous where uci named it), one removed
// removed again; a secret has no value to write.
func TestStageAgainWritesWhatTheEntryWrote(t *testing.T) {
	c := entry(1,
		set("lan", "ipaddr", "192.168.2.1", `"192.168.1.1"`),
		history.ChangeOp{Config: "network", Section: "lan", Op: "remove-option", Option: "mtu", Old: old(`"1400"`)},
		history.ChangeOp{Config: "network", Section: "lan", Op: "list-add", Option: "ports", Value: text("lan3"), Old: old(`["lan1"]`)},
		history.ChangeOp{Config: "network", Section: "cfg0a1b2c", Op: "add-section", Option: "route"},
		set("cfg0a1b2c", "target", "10.0.0.0/8", ""),
		history.ChangeOp{Config: "network", Section: "guest", Op: "add-section", Option: "interface"},
		history.ChangeOp{Config: "network", Section: "wan6", Op: "remove-section", Old: old(`{".type":"interface"}`)},
		history.ChangeOp{Config: "network", Section: "wan", Op: "set", Option: "password", Secret: true},
	)
	c.Status = history.ChangeRolledBack
	plan := forwardOf(c)
	want := []plugin.CommitOp{
		{Config: "network", Section: "lan", Values: map[string]any{"ipaddr": "192.168.2.1", "mtu": nil, "ports": []string{"lan1", "lan3"}}},
		{Config: "network", Type: "route", Values: map[string]any{"target": "10.0.0.0/8"}},
		{Config: "network", Section: "guest", Type: "interface", Values: map[string]any{}},
		{Config: "network", Section: "wan6", Delete: true},
	}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
	if !reflect.DeepEqual(plan.Secrets, []string{"network.wan.password"}) {
		t.Errorf("secrets = %v", plan.Secrets)
	}
}

// TestSinceThenIsWhatIsInForceAfterAPoint: newer applies in force, oldest
// first; a rolled-back or undone entry is not, and an undo of something after
// the point cancels with it.
func TestSinceThenIsWhatIsInForceAfterAPoint(t *testing.T) {
	undone := entry(3)
	undone.Status = history.ChangeUndone
	undo := entry(4)
	undo.Relation, undo.Related = "undo", 3
	rolled := entry(5)
	rolled.Status = history.ChangeRolledBack
	olderUndo := entry(6)
	olderUndo.Relation, olderUndo.Related = "undo", 1
	all := []history.Change{entry(7), olderUndo, rolled, undo, undone, entry(2), entry(1)}

	var got []int64
	for _, c := range sinceThen(all, entry(2)) {
		got = append(got, c.ID)
	}
	if !reflect.DeepEqual(got, []int64{6, 7}) {
		t.Fatalf("since 2 = %v, want 6 and 7", got)
	}
	if got := inForceAfter(all, entry(2)); !reflect.DeepEqual(got, []int64{4, 6, 7}) {
		t.Errorf("a return to 2 undoes %v, want 4, 6 and 7", got)
	}
}

// TestALaterChangeToTheSameOptionIsNamed: undoing writes over it.
func TestALaterChangeToTheSameOptionIsNamed(t *testing.T) {
	c := entry(1, set("lan", "mtu", "1400", `"1500"`))
	all := []history.Change{entry(4, set("lan", "proto", "dhcp", `"static"`)), entry(3, set("lan", "mtu", "9000", `"1400"`)), entry(2, set("wan", "mtu", "1", `"2"`)), c}
	var got []int64
	for _, later := range touchedAgain(all, c) {
		got = append(got, later.ID)
	}
	if !reflect.DeepEqual(got, []int64{3}) {
		t.Fatalf("touched again by %v, want 3", got)
	}
}

// actsServer is a signed-in server with config history in history.db, a
// plugin writing firewall, and what the shell stages recorded. stage is what
// uci reports staged when an apply reads it.
func actsServer(t *testing.T, stage map[string][][]string) (*Server, *history.Store, *[]uciWrite, *func()) {
	t.Helper()
	m := demoACLManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "firewall", Function: "write"})
	writes, confirms := []uciWrite{}, 0
	s := newServerWith(t, fakeBackend{access: true, writes: &writes, confirms: &confirms, changes: stage}, &fakeTransport{}, []plugin.Manifest{m})
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore, s.changeLog = store, store
	var window func()
	s.afterRollback = func(_ time.Duration, f func()) { window = f }
	return s, store, &writes, &window
}

func act(t *testing.T, s *Server, id int64, what string) *httptest.ResponseRecorder {
	t.Helper()
	return postPlugin(t, s, fmt.Sprintf("%s/%d", changesPath, id), url.Values{"act": {what}})
}

// TestUndoStagesWhatTheChangeReplaced: Undo stages the value the change
// replaced, under the page it was made on; applied and kept, it is an entry
// of its own, the undo of the first, which is undone.
func TestUndoStagesWhatTheChangeReplaced(t *testing.T) {
	s, store, writes, window := actsServer(t, map[string][][]string{"firewall": {{"set", "cfg0792bd", "target", "ACCEPT"}}})
	c := turnedOff(time.Now())
	c.OldRead = true
	id := record(t, store, c)

	if rec := act(t, s, id, "undo"); rec.Code != http.StatusSeeOther {
		t.Fatalf("undo = %d: %s", rec.Code, rec.Body)
	}
	want := []uciWrite{{"test-sid", "firewall", "cfg0792bd", map[string]any{"target": "ACCEPT"}}}
	if !reflect.DeepEqual(*writes, want) {
		t.Fatalf("staged %+v, want %+v", *writes, want)
	}
	if who, ok := s.authors.of("test-sid", optionAddress("firewall", "cfg0792bd", "target")); !ok || who.Page != "Firewall" {
		t.Errorf("staged as %+v, want under Firewall", who)
	}

	applyConfirmed(t, s, window)
	all, _ := store.Changes(t.Context(), 10)
	if len(all) != 2 || all[0].Relation != "undo" || all[0].Related != id {
		t.Fatalf("history = %+v, want the undo of %d", all, id)
	}
	if all[1].Status != history.ChangeUndone || all[1].UndoneBy != all[0].ID {
		t.Errorf("the change undone = %s by %d", all[1].Status, all[1].UndoneBy)
	}
}

// TestAChangeWithoutItsOldValuesCannotBeUndone: the drawer says why and
// offers no Undo, and an Undo posted anyway stages nothing.
func TestAChangeWithoutItsOldValuesCannotBeUndone(t *testing.T) {
	s, store, writes, _ := actsServer(t, nil)
	id := record(t, store, turnedOff(time.Now()))
	body := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id)).Body.String()
	if !strings.Contains(body, "cannot be undone") || strings.Contains(body, "Undo this change") {
		t.Errorf("drawer:\n%s", body)
	}
	if rec := act(t, s, id, "undo"); rec.Code != http.StatusConflict || len(*writes) != 0 {
		t.Fatalf("undo = %d, staged %+v", rec.Code, *writes)
	}
}

// TestARolledBackChangeIsStagedAgain: as it was written.
func TestARolledBackChangeIsStagedAgain(t *testing.T) {
	s, store, writes, _ := actsServer(t, nil)
	c := turnedOff(time.Now())
	c.Status, c.OldRead = history.ChangeRolledBack, true
	id := record(t, store, c)
	if body := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id)).Body.String(); !strings.Contains(body, "Stage this change again") {
		t.Fatalf("drawer should offer Stage again:\n%s", body)
	}
	if rec := act(t, s, id, "again"); rec.Code != http.StatusSeeOther {
		t.Fatalf("again = %d: %s", rec.Code, rec.Body)
	}
	if want := []uciWrite{{"test-sid", "firewall", "cfg0792bd", map[string]any{"target": "DROP"}}}; !reflect.DeepEqual(*writes, want) {
		t.Fatalf("staged %+v, want %+v", *writes, want)
	}
}

// TestReturnStagesWhatChangedSince: Since then lists what is set
// differently now, and Return stages it back.
func TestReturnStagesWhatChangedSince(t *testing.T) {
	s, store, writes, _ := actsServer(t, nil)
	first := turnedOff(time.Now().Add(-time.Hour))
	first.OldRead = true
	id := record(t, store, first)
	later := turnedOff(time.Now())
	later.OldRead = true
	later.Items[0].Ops[0].Value, later.Items[0].Ops[0].Old = text("REJECT"), json.RawMessage(`"DROP"`)
	record(t, store, later)

	body := getPanel(t, s, fmt.Sprintf("%s/%d?tab=since", changesPath, id)).Body.String()
	for _, want := range []string{">firewall.cfg0792bd.target<", ">REJECT<", ">DROP<", "Return to this point"} {
		if !strings.Contains(body, want) {
			t.Errorf("since then missing %q:\n%s", want, body)
		}
	}
	if rec := act(t, s, id, "return"); rec.Code != http.StatusSeeOther {
		t.Fatalf("return = %d: %s", rec.Code, rec.Body)
	}
	if want := []uciWrite{{"test-sid", "firewall", "cfg0792bd", map[string]any{"target": "DROP"}}}; !reflect.DeepEqual(*writes, want) {
		t.Fatalf("staged %+v, want %+v", *writes, want)
	}
}

// TestNothingSinceSaysSo.
func TestNothingSinceSaysSo(t *testing.T) {
	s, store, _, _ := actsServer(t, nil)
	id := record(t, store, turnedOff(time.Now()))
	if body := getPanel(t, s, fmt.Sprintf("%s/%d?tab=since", changesPath, id)).Body.String(); !strings.Contains(body, "Nothing has changed since.") {
		t.Errorf("since then:\n%s", body)
	}
}

// TestADiscardedStageForgetsWhatItWasToHistory: an apply after a discard is
// no undo.
func TestADiscardedStageForgetsWhatItWasToHistory(t *testing.T) {
	s, store, _, _ := actsServer(t, nil)
	c := turnedOff(time.Now())
	c.OldRead = true
	id := record(t, store, c)
	act(t, s, id, "undo")
	postPlugin(t, s, "/uci/discard", nil)
	if _, ok := s.relations.take("test-sid"); ok {
		t.Fatal("the discarded stage is still an undo")
	}
}

// TestReturnToTheStartingPointWritesWhatDiffers: each option back to what
// it held, a section made since removed, one removed since made again as it
// was; a secret stays.
func TestReturnToTheStartingPointWritesWhatDiffers(t *testing.T) {
	start := map[string]map[string]map[string]any{"network": {
		"lan":       {".type": "interface", ".index": 0.0, "mtu": "1500", "ports": []any{"lan1"}},
		"wan":       {".type": "interface", ".index": 1.0, "password": map[string]any{"secret": "aa"}},
		"cfg0a1b2c": {".type": "route", ".anonymous": true, ".index": 2.0, "target": "10.0.0.0/8"},
	}}
	now := map[string]map[string]map[string]any{"network": {
		"lan":   {".type": "interface", ".index": 0.0, "mtu": "9000", "ports": []any{"lan1", "lan2"}, "dns": "1.1.1.1"},
		"wan":   {".type": "interface", ".index": 1.0, "password": map[string]any{"secret": "bb"}},
		"guest": {".type": "interface", ".index": 2.0},
	}}
	plan := startPlan(start, now)
	want := []plugin.CommitOp{
		{Config: "network", Section: "lan", Values: map[string]any{"dns": nil, "mtu": "1500", "ports": []string{"lan1"}}},
		{Config: "network", Type: "route", Values: map[string]any{"target": "10.0.0.0/8"}},
		{Config: "network", Section: "guest", Delete: true},
	}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
	if !reflect.DeepEqual(plan.Secrets, []string{"network.wan.password"}) {
		t.Errorf("secrets = %v", plan.Secrets)
	}
	wantRows := []planRow{
		{"network.lan.dns", "1.1.1.1", "—"},
		{"network.lan.mtu", "9000", "1500"},
		{"network.lan.ports", "lan1 lan2", "lan1"},
		{"network.cfg0a1b2c", "—", "route"},
		{"network.guest", "interface", "—"},
	}
	if !reflect.DeepEqual(plan.Rows, wantRows) {
		t.Errorf("rows\n%v\nwant\n%v", plan.Rows, wantRows)
	}
}

// TestTheStartingPointClosesTheListingAndReturnsTheRouterToIt: the last
// row, whose drawer lists what differs now and stages it back.
func TestTheStartingPointClosesTheListingAndReturnsTheRouterToIt(t *testing.T) {
	s, store, writes, window := actsServer(t, map[string][][]string{"firewall": {{"set", "cfg0792bd", "target", "ACCEPT"}}})
	ctx := t.Context()
	_ = store.StartConfig(ctx, "firewall", json.RawMessage(`{"cfg0792bd":{".type":"rule","name":"Allow-Ping","target":"ACCEPT"}}`), time.Now().Add(-time.Hour))
	_ = store.SetConfigState(ctx, "firewall", json.RawMessage(`{"cfg0792bd":{".type":"rule","name":"Allow-Ping","target":"DROP"}}`), time.Now())
	c := turnedOff(time.Now())
	c.OldRead = true
	id := record(t, store, c)

	_, body := getSignedIn(t, s, changesPath)
	if last, start := strings.Index(body, fmt.Sprintf(`data-verso-row-id="%d"`, id)), strings.Index(body, `data-verso-row-id="start"`); start < 0 || start < last || !strings.Contains(body, ">Starting point<") {
		t.Fatalf("the starting point should close the listing (%d, %d)", last, start)
	}
	drawer := getPanel(t, s, changesPath+"/start").Body.String()
	for _, want := range []string{">firewall.cfg0792bd.target<", ">DROP<", ">ACCEPT<", "Return to the starting point"} {
		if !strings.Contains(drawer, want) {
			t.Errorf("starting point drawer missing %q:\n%s", want, drawer)
		}
	}
	if rec := postPlugin(t, s, changesPath+"/start", url.Values{"act": {"return"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("return = %d: %s", rec.Code, rec.Body)
	}
	if want := []uciWrite{{"test-sid", "firewall", "cfg0792bd", map[string]any{"target": "ACCEPT"}}}; !reflect.DeepEqual(*writes, want) {
		t.Fatalf("staged %+v, want %+v", *writes, want)
	}
	applyConfirmed(t, s, window)
	all := entries(t, store)
	if all[0].Relation != "return" || all[0].Related != 0 || all[1].Status != history.ChangeUndone {
		t.Fatalf("history = %+v", all)
	}
	if _, page := getSignedIn(t, s, changesPath); !strings.Contains(page, "back to the starting point") {
		t.Error("the return should say where it went")
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"encoding/json"
	"reflect"
	"testing"
)

func readStart(t *testing.T, store *Store) map[string]map[string]map[string]any {
	t.Helper()
	start, err := store.StartingPoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]map[string]any{}
	for config, raw := range start.Configs {
		var sections map[string]map[string]any
		if err := json.Unmarshal(raw, &sections); err != nil {
			t.Fatal(err)
		}
		out[config] = sections
	}
	return out
}

// TestTheStartingPointIsAConfigAsFirstRead: a later reading does not move
// it.
func TestTheStartingPointIsAConfigAsFirstRead(t *testing.T) {
	store, _ := open(t)
	for _, sections := range []string{`{"lan":{".type":"interface","mtu":"1500"}}`, `{"lan":{".type":"interface","mtu":"9000"}}`} {
		if err := store.StartConfig(ctx, "network", json.RawMessage(sections), minute(0)); err != nil {
			t.Fatal(err)
		}
	}
	if got := readStart(t, store)["network"]["lan"]["mtu"]; got != "1500" {
		t.Fatalf("starting mtu = %v, want 1500", got)
	}
}

// TestHistoryKeepsTheLatestAndFoldsTheRest: past the limit the oldest
// entries leave history, and the starting point moves on by what they did;
// one rolled back did nothing, and one undone did what it did before the
// entry that undid it.
func TestHistoryKeepsTheLatestAndFoldsTheRest(t *testing.T) {
	store, _ := open(t)
	if err := store.StartConfig(ctx, "network", json.RawMessage(`{
		"lan": {".type":"interface",".index":0,"mtu":"1500","ports":["lan1"],"ip6assign":"60"},
		"wan6":{".type":"interface",".index":1,"proto":"dhcpv6"}
	}`), minute(0)); err != nil {
		t.Fatal(err)
	}
	op := func(op, section, option string, value *string) ChangeOp {
		return ChangeOp{Config: "network", Section: section, Op: op, Option: option, Value: value}
	}
	entry := func(status string, ops ...ChangeOp) Change {
		return Change{Time: minute(1), Status: status, Items: []ChangeItem{{Page: "Interfaces", Verb: "changed", Words: "w", Ops: ops}}}
	}
	var ids []int64
	for _, c := range []Change{
		entry(ChangeApplied, op("set", "lan", "mtu", text("9000")), op("list-add", "lan", "ports", text("lan2")), op("remove-option", "lan", "ip6assign", nil)),
		entry(ChangeRolledBack, op("set", "lan", "mtu", text("1")), op("remove-section", "wan6", "", nil)),
		entry(ChangeUndone, op("add-section", "guest", "interface", nil), op("set", "guest", "proto", text("static")), op("remove-section", "wan6", "", nil)),
		entry(ChangeApplied, op("set", "lan", "mtu", text("1400"))),
	} {
		id, err := store.RecordChange(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := store.FoldChanges(ctx, 1); err != nil {
		t.Fatal(err)
	}
	left, _ := store.Changes(ctx, 10)
	if len(left) != 1 || left[0].ID != ids[3] {
		t.Fatalf("kept %+v, want only the latest", left)
	}
	want := map[string]map[string]any{
		"lan":   {".type": "interface", ".index": float64(0), "mtu": "9000", "ports": []any{"lan1", "lan2"}},
		"guest": {".type": "interface", ".name": "guest", ".anonymous": false, ".index": float64(2), "proto": "static"},
	}
	if got := readStart(t, store)["network"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("starting point\n%v\nwant\n%v", got, want)
	}
	start, _ := store.StartingPoint(ctx)
	if !start.Time.Equal(minute(1)) {
		t.Errorf("the starting point is from %v, want the last entry folded into it", start.Time)
	}
}

// TestFoldingUnderTheLimitKeepsEverything.
func TestFoldingUnderTheLimitKeepsEverything(t *testing.T) {
	store, _ := open(t)
	_, _ = store.RecordChange(ctx, applied(minute(0)))
	if err := store.FoldChanges(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if left, _ := store.Changes(ctx, 10); len(left) != 1 {
		t.Fatalf("kept %d, want 1", len(left))
	}
}

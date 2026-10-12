// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func text(s string) *string { return &s }

// applied is an apply as the shell records it: a firewall rule added and SSH
// moved to every interface, on two pages.
func applied(at time.Time) Change {
	return Change{
		Time: at, Status: ChangeApplied, User: "root", Address: "10.0.0.12", OldRead: true,
		Items: []ChangeItem{
			{Page: "Firewall", Plugin: "firewall", Verb: "added", Words: "Added the rule “Allow-SSH-WAN”.",
				SubjectKind: "rule", SubjectName: "Allow-SSH-WAN",
				Ops: []ChangeOp{
					{Config: "firewall", Section: "cfg0992bd", Op: "add-section", Option: "rule"},
					{Config: "firewall", Section: "cfg0992bd", Op: "set", Option: "name", Value: text("Allow-SSH-WAN")},
					{Config: "firewall", Section: "cfg0992bd", Op: "set", Option: "dest_port", Value: text("22")},
				}},
			{Page: "Access", Plugin: "system", Verb: "changed", Words: "dropbear.main.Interface = ''", Verbatim: true,
				Ops: []ChangeOp{
					{Config: "dropbear", Section: "main", Op: "set", Option: "Interface", Value: text(""), Old: json.RawMessage(`"lan"`)},
				}},
		},
	}
}

// readBack is the change recorded as id.
func readBack(t *testing.T, store *Store, id int64) Change {
	t.Helper()
	c, ok, err := store.Change(ctx, id)
	if err != nil || !ok {
		t.Fatalf("change %d: found %v, %v", id, ok, err)
	}
	return c
}

// TestChangesAreListedNewestFirst: the listing is the latest changes, newest
// first, each whole, up to the count asked for.
func TestChangesAreListedNewestFirst(t *testing.T) {
	store, _ := open(t)
	var ids []int64
	for i := range 3 {
		c := applied(minute(i))
		c.Note = fmt.Sprint(i)
		id, err := store.RecordChange(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	got, err := store.Changes(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids[2] || got[1].ID != ids[1] {
		t.Fatalf("listed %+v, want changes %d and %d", got, ids[2], ids[1])
	}
	want := applied(minute(1))
	want.ID, want.Note = ids[1], "1"
	if !reflect.DeepEqual(got[1], want) {
		t.Fatalf("listed\n%+v\nwant\n%+v", got[1], want)
	}
}

// TestAChangeNeverRecordedIsNotFound.
func TestAChangeNeverRecordedIsNotFound(t *testing.T) {
	store, _ := open(t)
	if _, ok, err := store.Change(ctx, 7); ok || err != nil {
		t.Fatalf("found %v, %v", ok, err)
	}
}

// TestAChangeIsRecordedWhole: every row and uci change keeps its order and
// its words, an empty value stays told from no value, and an old value stays
// told from none.
func TestAChangeIsRecordedWhole(t *testing.T) {
	store, _ := open(t)
	want := applied(minute(0))
	id, err := store.RecordChange(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	want.ID = id
	if got := readBack(t, store, id); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded\n%+v\nwant\n%+v", got, want)
	}
}

// TestASecretIsNeverStored: a change to a secret records that it changed and
// nothing of what it was or became, whatever the caller handed over.
func TestASecretIsNeverStored(t *testing.T) {
	store, _ := open(t)
	c := Change{Time: minute(0), Status: ChangeApplied, Items: []ChangeItem{{Page: "Wireless", Verb: "changed", Words: "w",
		Ops: []ChangeOp{{Config: "wireless", Section: "default_radio0", Op: "set", Option: "key", Value: text("hunter22"), Old: json.RawMessage(`"letmein1"`), Secret: true}}}}}
	id, err := store.RecordChange(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if op := readBack(t, store, id).Items[0].Ops[0]; !op.Secret || op.Value != nil || op.Old != nil {
		t.Fatalf("recorded %+v, want a secret with no values", op)
	}
}

// TestChangesTakeIncreasingIDs: each apply is its own entry, in the order
// recorded.
func TestChangesTakeIncreasingIDs(t *testing.T) {
	store, _ := open(t)
	first, err := store.RecordChange(ctx, applied(minute(0)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RecordChange(ctx, applied(minute(1)))
	if err != nil {
		t.Fatal(err)
	}
	if second <= first {
		t.Fatalf("ids %d then %d", first, second)
	}
}

// TestAChangeSaysWhetherWhatItReplacedWasRead: an entry whose old values
// could not be read says so, so nothing is undone to a value it never had.
func TestAChangeSaysWhetherWhatItReplacedWasRead(t *testing.T) {
	store, _ := open(t)
	for _, read := range []bool{true, false} {
		c := applied(minute(0))
		c.OldRead = read
		id, err := store.RecordChange(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if got := readBack(t, store, id).OldRead; got != read {
			t.Errorf("old read = %v, want %v", got, read)
		}
	}
}

// TestUndoneChangesNameWhatUndidThem: only an entry in force is undone, and
// it names the entry that undid it.
func TestUndoneChangesNameWhatUndidThem(t *testing.T) {
	store, _ := open(t)
	kept, _ := store.RecordChange(ctx, applied(minute(0)))
	back := applied(minute(1))
	back.Status = ChangeRolledBack
	rolled, _ := store.RecordChange(ctx, back)
	undo, _ := store.RecordChange(ctx, applied(minute(2)))
	if err := store.MarkUndone(ctx, []int64{kept, rolled}, undo); err != nil {
		t.Fatal(err)
	}
	if c := readBack(t, store, kept); c.Status != ChangeUndone || c.UndoneBy != undo {
		t.Errorf("kept = %s by %d, want undone by %d", c.Status, c.UndoneBy, undo)
	}
	if c := readBack(t, store, rolled); c.Status != ChangeRolledBack || c.UndoneBy != 0 {
		t.Errorf("rolled back = %s by %d, want left as it was", c.Status, c.UndoneBy)
	}
}

// TestAConfigIsKeptAsLastRead: unknown until first read, then as it was last
// read, the earlier reading replaced.
func TestAConfigIsKeptAsLastRead(t *testing.T) {
	store, _ := open(t)
	if _, ok, err := store.ConfigState(ctx, "network"); ok || err != nil {
		t.Fatalf("an unread config: found %v, %v", ok, err)
	}
	for _, sections := range []string{`{"lan":{".type":"interface"}}`, `{"wan":{".type":"interface"}}`} {
		if err := store.SetConfigState(ctx, "network", json.RawMessage(sections), minute(0)); err != nil {
			t.Fatal(err)
		}
	}
	got, ok, err := store.ConfigState(ctx, "network")
	if err != nil || !ok || string(got) != `{"wan":{".type":"interface"}}` {
		t.Fatalf("network = %s, %v, %v", got, ok, err)
	}
}

// TestEveryConfigIsReadAsLastKept.
func TestEveryConfigIsReadAsLastKept(t *testing.T) {
	store, _ := open(t)
	_ = store.SetConfigState(ctx, "network", json.RawMessage(`{"lan":{}}`), minute(0))
	_ = store.SetConfigState(ctx, "firewall", json.RawMessage(`{"r":{}}`), minute(0))
	states, err := store.ConfigStates(ctx)
	if err != nil || len(states) != 2 || string(states["firewall"]) != `{"r":{}}` {
		t.Fatalf("states = %v, %v", states, err)
	}
}

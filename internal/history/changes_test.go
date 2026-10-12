// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func text(s string) *string { return &s }

// applied is an apply as the shell records it: a firewall rule added and SSH
// moved to every interface, on two pages.
func applied(at time.Time) Change {
	return Change{
		Time: at, Status: ChangeApplied, User: "root", Address: "10.0.0.12",
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

// readBack reads the change recorded as id straight from the file.
func readBack(t *testing.T, store *Store, id int64) Change {
	t.Helper()
	c := Change{ID: id}
	var at int64
	var related, undoneBy sql.NullInt64
	if err := store.db.QueryRow(`SELECT time, status, user, address, note, relation, related, undone_by FROM changes WHERE id = ?`, id).
		Scan(&at, &c.Status, &c.User, &c.Address, &c.Note, &c.Relation, &related, &undoneBy); err != nil {
		t.Fatal(err)
	}
	c.Time, c.Related, c.UndoneBy = time.Unix(at, 0), related.Int64, undoneBy.Int64
	items, err := store.db.Query(`SELECT page, plugin, verb, words, verbatim, subject_kind, subject_name FROM change_items WHERE change_id = ? ORDER BY position`, id)
	if err != nil {
		t.Fatal(err)
	}
	for items.Next() {
		var item ChangeItem
		if err := items.Scan(&item.Page, &item.Plugin, &item.Verb, &item.Words, &item.Verbatim, &item.SubjectKind, &item.SubjectName); err != nil {
			t.Fatal(err)
		}
		c.Items = append(c.Items, item)
	}
	_ = items.Close()
	ops, err := store.db.Query(`SELECT item, config, section, op, option, value, old, secret FROM change_ops WHERE change_id = ? ORDER BY item, position`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ops.Close() }()
	for ops.Next() {
		var item int
		var op ChangeOp
		var value, old sql.NullString
		if err := ops.Scan(&item, &op.Config, &op.Section, &op.Op, &op.Option, &value, &old, &op.Secret); err != nil {
			t.Fatal(err)
		}
		if value.Valid {
			op.Value = &value.String
		}
		if old.Valid {
			op.Old = json.RawMessage(old.String)
		}
		c.Items[item].Ops = append(c.Items[item].Ops, op)
	}
	return c
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// A change's status.
const (
	ChangeApplied    = "applied"
	ChangeRolledBack = "rolledback"
	ChangeUndone     = "undone"
)

// Change is one apply, one edit written straight to the config, or one commit
// the router made itself (ADR-021). User and Address are empty for a direct
// edit and for the router's own commit, unless an operator's act made it.
// Cause is what made the router commit, in English, and CausePlugin the
// plugin whose catalog words it, empty for the shell's words. Related is the
// change that Relation names; UndoneBy the change that undid this one; 0 is
// none. OldRead says what each change replaced was read before the apply, so
// Old is what was there, nil included; without it Old knows nothing.
type Change struct {
	ID          int64
	Time        time.Time
	Status      string
	User        string
	Address     string
	Note        string
	Relation    string
	Related     int64
	UndoneBy    int64
	OldRead     bool
	Cause       string
	CausePlugin string
	Items       []ChangeItem
}

// ChangeItem is a row as the review drawer read it: the owning plugin's words
// for it, or the raw uci line (Verbatim); what happened, as the drawer's verb;
// and the object it is about, when its plugin named one.
type ChangeItem struct {
	Page        string
	Plugin      string
	Verb        string
	Words       string
	Verbatim    bool
	SubjectKind string
	SubjectName string
	Ops         []ChangeOp
}

// ChangeOp is one uci change, in the describe hook's vocabulary (set,
// add-section, remove-option, remove-section, list-add, list-del, file). Value
// is what the option became; nil is no value, which an empty string is not.
// Old is what was committed before, as JSON in the shape rpcd's uci get has
// it: the option's string or list for a change to an option, the whole
// section (".type" and its options) for a section removed; nil where nothing
// was. A Secret keeps neither.
type ChangeOp struct {
	Config  string
	Section string
	Op      string
	Option  string
	Value   *string
	Old     json.RawMessage
	Secret  bool
}

// RecordChange stores c in one transaction and answers its id. A secret's
// values are dropped here, whatever the caller handed over.
func (s *Store) RecordChange(ctx context.Context, c Change) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO changes (time, status, user, address, note, relation, related, undone_by, old_read, cause, cause_plugin) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Time.Unix(), c.Status, c.User, c.Address, c.Note, c.Relation, nullID(c.Related), nullID(c.UndoneBy), c.OldRead, c.Cause, c.CausePlugin)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, item := range c.Items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO change_items (change_id, position, page, plugin, verb, words, verbatim, subject_kind, subject_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i, item.Page, item.Plugin, item.Verb, item.Words, item.Verbatim, item.SubjectKind, item.SubjectName); err != nil {
			return 0, err
		}
		for j, op := range item.Ops {
			value, old := op.Value, oldText(op.Old)
			if op.Secret {
				value, old = nil, nil
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO change_ops (change_id, item, position, config, section, op, option, value, old, secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, i, j, op.Config, op.Section, op.Op, op.Option, value, old, op.Secret); err != nil {
				return 0, err
			}
		}
	}
	return id, tx.Commit()
}

// oldText stores no old value as NULL, and any other as its JSON text.
func oldText(old json.RawMessage) *string {
	if old == nil {
		return nil
	}
	s := string(old)
	return &s
}

// nullID stores 0, no change, as NULL.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// Changes is the latest limit changes, newest first, each whole.
func (s *Store) Changes(ctx context.Context, limit int) ([]Change, error) {
	return s.changes(ctx, `ORDER BY id DESC LIMIT ?`, limit)
}

// Change is the change recorded as id, and false where there is none.
func (s *Store) Change(ctx context.Context, id int64) (Change, bool, error) {
	found, err := s.changes(ctx, `WHERE id = ?`, id)
	if err != nil || len(found) == 0 {
		return Change{}, false, err
	}
	return found[0], true, nil
}

// changes reads the changes the query's tail selects, in its order, with
// their rows and each row's uci changes.
func (s *Store) changes(ctx context.Context, tail string, args ...any) ([]Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, time, status, user, address, note, relation, related, undone_by, old_read, cause, cause_plugin FROM changes `+tail, args...)
	if err != nil {
		return nil, err
	}
	var list []Change
	index := map[int64]int{}
	lowest := int64(0)
	for rows.Next() {
		var c Change
		var at int64
		var related, undoneBy sql.NullInt64
		if err := rows.Scan(&c.ID, &at, &c.Status, &c.User, &c.Address, &c.Note, &c.Relation, &related, &undoneBy, &c.OldRead, &c.Cause, &c.CausePlugin); err != nil {
			_ = rows.Close()
			return nil, err
		}
		c.Time, c.Related, c.UndoneBy = time.Unix(at, 0), related.Int64, undoneBy.Int64
		index[c.ID] = len(list)
		list = append(list, c)
		if lowest == 0 || c.ID < lowest {
			lowest = c.ID
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil || len(list) == 0 {
		return list, err
	}
	if err := s.changeItems(ctx, list, index, lowest); err != nil {
		return nil, err
	}
	return list, s.changeOps(ctx, list, index, lowest)
}

// changeItems fills in the rows of the changes listed, from lowest on.
func (s *Store) changeItems(ctx context.Context, list []Change, index map[int64]int, lowest int64) error {
	rows, err := s.db.QueryContext(ctx, `SELECT change_id, page, plugin, verb, words, verbatim, subject_kind, subject_name FROM change_items WHERE change_id >= ? ORDER BY change_id, position`, lowest)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var item ChangeItem
		if err := rows.Scan(&id, &item.Page, &item.Plugin, &item.Verb, &item.Words, &item.Verbatim, &item.SubjectKind, &item.SubjectName); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			list[i].Items = append(list[i].Items, item)
		}
	}
	return rows.Err()
}

// changeOps fills in each row's uci changes, from lowest on.
func (s *Store) changeOps(ctx context.Context, list []Change, index map[int64]int, lowest int64) error {
	rows, err := s.db.QueryContext(ctx, `SELECT change_id, item, config, section, op, option, value, old, secret FROM change_ops WHERE change_id >= ? ORDER BY change_id, item, position`, lowest)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var item int
		var op ChangeOp
		var value, old sql.NullString
		if err := rows.Scan(&id, &item, &op.Config, &op.Section, &op.Op, &op.Option, &value, &old, &op.Secret); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || item >= len(list[i].Items) {
			continue
		}
		if value.Valid {
			op.Value = &value.String
		}
		if old.Valid {
			op.Old = json.RawMessage(old.String)
		}
		list[i].Items[item].Ops = append(list[i].Items[item].Ops, op)
	}
	return rows.Err()
}

// MarkUndone records the changes in force among ids as undone by the change
// by. One rolled back, or already undone, stays as it is.
func (s *Store) MarkUndone(ctx context.Context, ids []int64, by int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE changes SET status = ?, undone_by = ? WHERE id = ? AND status = ?`,
			ChangeUndone, by, id, ChangeApplied); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ConfigState is config as config history last read it, and false before
// the first reading.
func (s *Store) ConfigState(ctx context.Context, config string) (json.RawMessage, bool, error) {
	var sections string
	err := s.db.QueryRowContext(ctx, `SELECT sections FROM config_state WHERE config = ?`, config).Scan(&sections)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(sections), true, nil
}

// SetConfigState keeps config's sections as config history now reads them,
// written at at.
func (s *Store) SetConfigState(ctx context.Context, config string, sections json.RawMessage, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config_state (config, sections, time) VALUES (?, ?, ?) ON CONFLICT (config) DO UPDATE SET sections = excluded.sections, time = excluded.time`,
		config, string(sections), at.Unix())
	return err
}

// ConfigStates is every config as config history last read it.
func (s *Store) ConfigStates(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT config, sections FROM config_state`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	states := map[string]json.RawMessage{}
	for rows.Next() {
		var config, sections string
		if err := rows.Scan(&config, &sections); err != nil {
			return nil, err
		}
		states[config] = json.RawMessage(sections)
	}
	return states, rows.Err()
}

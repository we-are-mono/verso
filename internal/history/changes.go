// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"time"
)

// A change's status.
const (
	ChangeApplied    = "applied"
	ChangeRolledBack = "rolledback"
)

// Change is one apply, or one edit written straight to the config (ADR-021).
// User and Address are empty for a direct edit. Related is the change that
// Relation names; UndoneBy the change that undid this one; 0 is none.
type Change struct {
	ID       int64
	Time     time.Time
	Status   string
	User     string
	Address  string
	Note     string
	Relation string
	Related  int64
	UndoneBy int64
	Items    []ChangeItem
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
// is what the option became and Old what it replaced; nil is no value, which
// an empty string is not. A Secret keeps neither.
type ChangeOp struct {
	Config  string
	Section string
	Op      string
	Option  string
	Value   *string
	Old     *string
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
		`INSERT INTO changes (time, status, user, address, note, relation, related, undone_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Time.Unix(), c.Status, c.User, c.Address, c.Note, c.Relation, nullID(c.Related), nullID(c.UndoneBy))
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
			value, old := op.Value, op.Old
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

// nullID stores 0, no change, as NULL.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

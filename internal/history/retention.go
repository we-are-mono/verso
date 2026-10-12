// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"slices"
	"time"
)

// Config history keeps the latest entries, as many as it is told to, and
// beside them the starting point: each config as it stood before the oldest
// entry kept (ADR-021). An entry leaving history moves the starting point on
// by what it did, so Return to the starting point stays true.

// StartingPoint is each config as it stood before the oldest entry kept,
// its sections in the shape verso-rpcd spools them, and the moment it holds.
type StartingPoint struct {
	Time    time.Time
	Configs map[string]json.RawMessage
}

// StartConfig makes config's reading its starting point if it has none: the
// config as config history first finds it.
func (s *Store) StartConfig(ctx context.Context, config string, sections json.RawMessage, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config_start (config, sections, time) VALUES (?, ?, ?) ON CONFLICT (config) DO NOTHING`,
		config, string(sections), at.Unix())
	return err
}

// StartingPoint is the starting point of every config history has read;
// empty before the first.
func (s *Store) StartingPoint(ctx context.Context) (StartingPoint, error) {
	start := StartingPoint{Configs: map[string]json.RawMessage{}}
	rows, err := s.db.QueryContext(ctx, `SELECT config, sections, time FROM config_start`)
	if err != nil {
		return start, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var config, sections string
		var at int64
		if err := rows.Scan(&config, &sections, &at); err != nil {
			return start, err
		}
		start.Configs[config] = json.RawMessage(sections)
		if t := time.Unix(at, 0); t.After(start.Time) {
			start.Time = t
		}
	}
	return start, rows.Err()
}

// FoldChanges keeps the latest keep entries and folds every older one into
// the starting point, oldest first: what an entry that held did is applied
// to it, and the entry leaves history. One rolled back changed nothing.
func (s *Store) FoldChanges(ctx context.Context, keep int) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM changes`).Scan(&count); err != nil {
		return err
	}
	if count <= keep {
		return nil
	}
	oldest, err := s.changes(ctx, `ORDER BY id LIMIT ?`, count-keep)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range oldest {
		if c.Status != ChangeRolledBack {
			if err := foldInto(ctx, tx, c); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`UPDATE changes SET related = NULL WHERE related = ?`,
			`UPDATE changes SET undone_by = NULL WHERE undone_by = ?`,
			`DELETE FROM change_ops WHERE change_id = ?`,
			`DELETE FROM change_items WHERE change_id = ?`,
			`DELETE FROM changes WHERE id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, statement, c.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// foldInto applies c's changes to the starting point of each config it
// changed that has one.
func foldInto(ctx context.Context, tx *sql.Tx, c Change) error {
	byConfig := map[string][]ChangeOp{}
	var order []string
	for _, item := range c.Items {
		for _, op := range item.Ops {
			if _, seen := byConfig[op.Config]; !seen {
				order = append(order, op.Config)
			}
			byConfig[op.Config] = append(byConfig[op.Config], op)
		}
	}
	for _, config := range order {
		var text string
		err := tx.QueryRowContext(ctx, `SELECT sections FROM config_start WHERE config = ?`, config).Scan(&text)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		var sections map[string]map[string]any
		if err := json.Unmarshal([]byte(text), &sections); err != nil {
			return err
		}
		applyOps(sections, byConfig[config])
		moved, err := json.Marshal(sections)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE config_start SET sections = ?, time = ? WHERE config = ?`, string(moved), c.Time.Unix(), config); err != nil {
			return err
		}
	}
	return nil
}

// anonymousName is how uci names a section that has no name of its own.
var anonymousName = regexp.MustCompile(`^cfg[0-9a-f]{6}$`)

// applyOps does ops to sections, as uci would have: a value set or removed,
// a list's value added or taken, a section made or removed. A secret's
// change, which keeps no value, and a file's leave sections as they are.
// Doing a change twice leaves what doing it once did.
func applyOps(sections map[string]map[string]any, ops []ChangeOp) {
	for _, op := range ops {
		section := sections[op.Section]
		value := ""
		if op.Value != nil {
			value = *op.Value
		}
		switch {
		case op.Secret || op.Op == "file":
		case op.Op == "add-section":
			if section == nil {
				sections[op.Section] = map[string]any{".type": op.Option, ".name": op.Section,
					".anonymous": anonymousName.MatchString(op.Section), ".index": float64(len(sections))}
			}
		case op.Op == "remove-section":
			delete(sections, op.Section)
		case section == nil:
		case op.Op == "set":
			section[op.Option] = value
		case op.Op == "remove-option":
			delete(section, op.Option)
		case op.Op == "list-add":
			list, _ := section[op.Option].([]any)
			if !slices.Contains(list, any(value)) {
				section[op.Option] = append(list, value)
			}
		case op.Op == "list-del":
			list, _ := section[op.Option].([]any)
			section[op.Option] = slices.DeleteFunc(slices.Clone(list), func(v any) bool { return v == value })
		}
	}
}

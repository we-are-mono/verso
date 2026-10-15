// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Config history keeps no secret's value (ADR-021), in a change to it or in
// any copy: a section kept whole, the raw line a row is kept as, a config as
// last read. The secrets are named by config.option, in the shared list and
// each installed plugin's manifest, so they are the shell's to say.

// NotKept stands in a section kept whole for a secret's value: the option was
// there, what it held is not kept.
func NotKept() map[string]any { return map[string]any{"secret": true} }

// WithoutSecrets is section, of config, with the value of every option
// secrets names (config.option) not kept, and whether any was. A value
// already not kept, or a fingerprint, stays as it is.
func WithoutSecrets(config string, section map[string]any, secrets map[string]bool) (map[string]any, bool) {
	kept := make(map[string]any, len(section))
	scrubbed := false
	for option, value := range section {
		if _, gone := value.(map[string]any); !gone && secrets[config+"."+option] {
			value, scrubbed = NotKept(), true
		}
		kept[option] = value
	}
	return kept, scrubbed
}

// ErrNotRewritten is a scrub's answer when rows are scrubbed but the file
// still holds what they held: the rewrite failed, and every scrub makes it
// again until one succeeds.
var ErrNotRewritten = errors.New("history.db not rewritten")

// ScrubSecrets takes out of config history every value of a secret it holds:
// in a section removed, in a change to an option secrets names, in the words
// of a verbatim row for a secret's change, which become masked(op), and in
// each config as last read and as it stood at the starting point. It answers
// how many rows it wrote. What is scrubbed stays so: scrubbing again with the
// same secrets writes nothing, and with more scrubs only what they add.
//
// A row written leaves what it held in the pages SQLite freed, and in the
// log, so a scrub that wrote any rewrites the file whole (rewrite). The rows
// commit first, and with them the rewrite they owe, so a rewrite that fails
// or never finishes is made by the next scrub, though it writes no row; the
// error then is ErrNotRewritten. A history written since keeps no secret, so
// the deletes it makes free none, and secure_delete would only add writes to
// the flash.
func (s *Store) ScrubSecrets(ctx context.Context, secrets map[string]bool, masked func(ChangeOp) string) (int, error) {
	written, owed, err := s.scrubRows(ctx, secrets, masked)
	if err != nil {
		return 0, fmt.Errorf("config history not scrubbed of secrets: %w", err)
	}
	if !owed {
		return 0, nil
	}
	return written, s.rewrite(ctx)
}

// scrubRows writes the rows ScrubSecrets scrubs, and with them the rewrite
// they owe, in one transaction. It answers how many it wrote and whether the
// file is owed a rewrite: by them, or by a scrub before whose rewrite did not
// succeed. A scrub with no row to write writes nothing.
func (s *Store) scrubRows(ctx context.Context, secrets map[string]bool, masked func(ChangeOp) string) (int, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()
	written := 0
	for _, scrub := range []func() (int, error){
		func() (int, error) { return scrubRemovedSections(ctx, tx, secrets) },
		func() (int, error) { return scrubSecretOps(ctx, tx, secrets) },
		func() (int, error) { return scrubSecretWords(ctx, tx, masked) },
		func() (int, error) { return scrubReadings(ctx, tx, "config_state", secrets) },
		func() (int, error) { return scrubReadings(ctx, tx, "config_start", secrets) },
	} {
		n, err := scrub()
		if err != nil {
			return 0, false, err
		}
		written += n
	}
	if written == 0 {
		var owed bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rewrite_owed)`).Scan(&owed)
		return 0, owed, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rewrite_owed (id) VALUES (1)`); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return written, true, nil
}

// rewrite copies the file's live rows into a file of its own, which leaves no
// freed page, and empties the log, which held the pages as they were. Only
// then is the rewrite no longer owed, and the log is emptied again of the row
// saying so, which leaves it as a scrub with no row to write finds it, empty,
// so that scrub's read writes nothing either.
func (s *Store) rewrite(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("%w: %w", ErrNotRewritten, err)
	}
	if err := s.emptyLog(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrNotRewritten, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rewrite_owed`); err != nil {
		return fmt.Errorf("history.db rewritten, and still owing the rewrite: %w", err)
	}
	if err := s.emptyLog(ctx); err != nil {
		return fmt.Errorf("history.db rewritten: %w", err)
	}
	return nil
}

// emptyLog moves the log's pages into the file and truncates it.
func (s *Store) emptyLog(ctx context.Context) error {
	var busy, logged, moved int
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logged, &moved); err != nil {
		return fmt.Errorf("log not emptied: %w", err)
	}
	if busy != 0 {
		return errors.New("log not emptied: a reader holds it")
	}
	return nil
}

// opKey is one row of change_ops.
type opKey struct {
	id             int64
	item, position int
}

// scrubRemovedSections keeps each section removed without its secrets.
func scrubRemovedSections(ctx context.Context, tx *sql.Tx, secrets map[string]bool) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT change_id, item, position, config, old FROM change_ops WHERE op = 'remove-section' AND old IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	updates := map[opKey]string{}
	for rows.Next() {
		var key opKey
		var config, old string
		if err := rows.Scan(&key.id, &key.item, &key.position, &config, &old); err != nil {
			_ = rows.Close()
			return 0, err
		}
		var section map[string]any
		if json.Unmarshal([]byte(old), &section) != nil {
			continue
		}
		kept, scrubbed := WithoutSecrets(config, section, secrets)
		if !scrubbed {
			continue
		}
		encoded, err := json.Marshal(kept)
		if err != nil {
			_ = rows.Close()
			return 0, err
		}
		updates[key] = string(encoded)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	for key, old := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE change_ops SET old = ? WHERE change_id = ? AND item = ? AND position = ?`,
			old, key.id, key.item, key.position); err != nil {
			return 0, err
		}
	}
	return len(updates), nil
}

// scrubSecretOps marks each change to an option secrets names as a secret's,
// which keeps no value.
func scrubSecretOps(ctx context.Context, tx *sql.Tx, secrets map[string]bool) (int, error) {
	written := 0
	for name := range secrets {
		config, option, ok := strings.Cut(name, ".")
		if !ok {
			continue
		}
		result, err := tx.ExecContext(ctx,
			`UPDATE change_ops SET secret = 1, value = NULL, old = NULL
			 WHERE config = ? AND option = ? AND op IN ('set', 'list-add', 'list-del', 'remove-option')
			 AND (secret = 0 OR value IS NOT NULL OR old IS NOT NULL)`, config, option)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		written += int(n)
	}
	return written, nil
}

// scrubSecretWords words each verbatim row for a secret's change as masked
// has it. A row a plugin described keeps its words.
func scrubSecretWords(ctx context.Context, tx *sql.Tx, masked func(ChangeOp) string) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT i.change_id, i.position, i.words, o.config, o.section, o.op, o.option
		FROM change_items i JOIN change_ops o ON o.change_id = i.change_id AND o.item = i.position
		WHERE i.verbatim = 1 AND o.secret = 1`)
	if err != nil {
		return 0, err
	}
	updates := map[opKey]string{}
	for rows.Next() {
		var key opKey
		var words string
		var op ChangeOp
		if err := rows.Scan(&key.id, &key.item, &words, &op.Config, &op.Section, &op.Op, &op.Option); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if line := masked(op); line != words {
			updates[key] = line
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	for key, words := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE change_items SET words = ? WHERE change_id = ? AND position = ?`, words, key.id, key.item); err != nil {
			return 0, err
		}
	}
	return len(updates), nil
}

// scrubReadings keeps each config in table, config_state or config_start,
// without its secrets.
func scrubReadings(ctx context.Context, tx *sql.Tx, table string, secrets map[string]bool) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT config, sections FROM `+table)
	if err != nil {
		return 0, err
	}
	updates := map[string]string{}
	for rows.Next() {
		var config, text string
		if err := rows.Scan(&config, &text); err != nil {
			_ = rows.Close()
			return 0, err
		}
		var sections map[string]map[string]any
		if json.Unmarshal([]byte(text), &sections) != nil {
			continue
		}
		changed := false
		for name, section := range sections {
			kept, scrubbed := WithoutSecrets(config, section, secrets)
			sections[name], changed = kept, changed || scrubbed
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(sections)
		if err != nil {
			_ = rows.Close()
			return 0, err
		}
		updates[config] = string(encoded)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	for config, sections := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET sections = ? WHERE config = ?`, sections, config); err != nil {
			return 0, err
		}
	}
	return len(updates), nil
}

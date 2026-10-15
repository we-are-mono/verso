// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import "context"

// RenameSeries moves each old series in renames, every point at every
// resolution, to its new name, in one transaction. Where the new name already
// holds points, they stay and the old series' others join them. An old name
// the file does not hold changes nothing, so a rename asked again is free.
func (s *Store) RenameSeries(ctx context.Context, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for old, name := range renames {
		var held bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM series WHERE name = ?)`, old).Scan(&held); err != nil {
			return err
		}
		if !held || old == name {
			continue
		}
		for _, step := range []string{
			`INSERT OR IGNORE INTO series (name) VALUES (?2)`,
			`INSERT OR IGNORE INTO points (series_id, resolution, time, average, peak)
			 SELECT (SELECT id FROM series WHERE name = ?2), resolution, time, average, peak
			 FROM points WHERE series_id = (SELECT id FROM series WHERE name = ?1)`,
			`DELETE FROM points WHERE series_id = (SELECT id FROM series WHERE name = ?1)`,
			`DELETE FROM series WHERE name = ?1`,
		} {
			if _, err := tx.ExecContext(ctx, step, old, name); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

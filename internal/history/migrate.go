// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// The file's shape is a list of steps (ADR-019), one SQL file each in
// migrations/, named NNN_what.sql: step N brings a file to version N, kept in
// PRAGMA user_version. Every shape a shell has ever written is a step, so any
// file a router holds has a way forward. A shape changes only by a file added
// with the next number, never by editing one a router may already have
// taken, and never by rebuilding the file, because events and the alerts'
// record cannot be measured again.
const firstStep = 1

//go:embed migrations/*.sql
var migrations embed.FS

var steps, stepsErr = loadSteps(migrations)

// latest is the version a file is at once it has taken every step.
var latest = firstStep + len(steps) - 1

// loadSteps reads the steps in migrations/ in order. They must be numbered
// one by one from firstStep, or a version would mean different shapes on
// different routers.
func loadSteps(files fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(files, "migrations")
	if err != nil {
		return nil, fmt.Errorf("history: migrations: %w", err)
	}
	loaded := make([]string, 0, len(entries))
	for i, entry := range entries {
		name := entry.Name()
		number, _, found := strings.Cut(name, "_")
		n, err := strconv.Atoi(number)
		if !found || err != nil || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("history: migration %s is not named NNN_what.sql", name)
		}
		if n != firstStep+i {
			return nil, fmt.Errorf("history: migration %s is numbered %d, want %d", name, n, firstStep+i)
		}
		data, err := fs.ReadFile(files, path.Join("migrations", name))
		if err != nil {
			return nil, fmt.Errorf("history: migration %s: %w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("history: migration %s is empty", name)
		}
		loaded = append(loaded, string(data))
	}
	return loaded, nil
}

// shape brings the file open as db to the latest version, taking the steps it
// lacks. A file newer than latest is left as it is and refused, so a
// downgraded shell never throws away what an upgrade reads again; the caller
// keeps the history in memory.
func shape(db *sql.DB) error {
	if stepsErr != nil {
		return stepsErr
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > latest {
		return fmt.Errorf("history: the file is at version %d, newer than the %d this build reads", version, latest)
	}
	return migrate(db, version, steps)
}

// migrate takes each of steps a file at version from has not taken. A step
// commits together with the version it reaches, so a step that fails leaves
// the file at the version before it, to be tried again on the next open.
func migrate(db *sql.DB, from int, steps []string) error {
	for i, step := range steps {
		to := firstStep + i
		if to <= from {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(step); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("history: step %d: %w", to, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, to)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("history: step %d: %w", to, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("history: step %d: %w", to, err)
		}
	}
	return nil
}

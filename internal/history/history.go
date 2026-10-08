// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package history is the router's memory (ADR-019): measurements at four
// resolutions and the events daemons announced, in one SQLite file the shell
// alone writes. Minute points collect in memory and reach the file in one
// transaction per flush, so the flash sees a write every quarter of an hour,
// not every minute.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const driver = "sqlite"

// schema is the file's layout; a file of another version is rebuilt, not
// migrated. auto_vacuum must be set before the first table exists.
const (
	schemaVersion = 2
	schema        = `
PRAGMA auto_vacuum = INCREMENTAL;
CREATE TABLE series (
	id   INTEGER PRIMARY KEY,
	name TEXT    NOT NULL UNIQUE
);
CREATE TABLE points (
	series_id  INTEGER NOT NULL REFERENCES series (id),
	resolution INTEGER NOT NULL,
	time       INTEGER NOT NULL,
	average    REAL    NOT NULL,
	peak       REAL    NOT NULL,
	PRIMARY KEY (series_id, resolution, time)
) WITHOUT ROWID;
CREATE TABLE events (
	time    INTEGER NOT NULL,
	kind    TEXT    NOT NULL,
	subject TEXT    NOT NULL,
	detail  TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX events_time ON events (time);
PRAGMA user_version = 2;`
)

// Resolution is a point's period in seconds.
type Resolution int64

const (
	Minute  Resolution = 60
	Quarter Resolution = 15 * 60
	Hour    Resolution = 60 * 60
	Day     Resolution = 24 * 60 * 60
)

func (r Resolution) String() string {
	switch r {
	case Minute:
		return "minute"
	case Quarter:
		return "quarter"
	case Hour:
		return "hour"
	case Day:
		return "day"
	}
	return fmt.Sprintf("%ds", int64(r))
}

// ladder is each resolution with how long it is kept, finest first; each rolls
// up from the one before it.
var ladder = []struct {
	res  Resolution
	keep time.Duration
}{
	{Minute, 48 * time.Hour},
	{Quarter, 31 * 24 * time.Hour},
	{Hour, 90 * 24 * time.Hour},
	{Day, 400 * 24 * time.Hour},
}

// eventsKeep is how long events last: as long as the daily points they explain.
const eventsKeep = 400 * 24 * time.Hour

// Point is one period of a series: its average and its highest reading. Time
// is the period's start.
type Point struct {
	Series  string
	Time    time.Time
	Average float64
	Peak    float64
}

// Event is a fact a daemon or the kernel announced. Detail is JSON, or empty.
type Event struct {
	Time    time.Time
	Kind    string
	Subject string
	Detail  string
}

// Store is the open history file. Safe for concurrent use.
type Store struct {
	db *sql.DB

	mu      sync.Mutex
	pending []Point
}

// Open opens or creates the history file at path. The directory must exist:
// creating it is root's, and an unwritable one is the caller's to report.
func Open(path string) (*Store, error) {
	db, err := connect(path)
	if err != nil {
		return nil, err
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, err
	}
	if version != schemaVersion {
		if db, err = rebuild(db, path, version); err != nil {
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

// connect opens one connection: the shell is the only writer, and one
// connection keeps the pragmas on every statement.
func connect(path string) (*sql.DB, error) {
	db, err := sql.Open(driver, "file:"+path+
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// rebuild replaces a file of another schema version (0 is a new, empty file).
func rebuild(db *sql.DB, path string, version int) (*sql.DB, error) {
	if version != 0 {
		_ = db.Close()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
		var err error
		if db, err = connect(path); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("history: create schema: %w", err)
	}
	return db, nil
}

// Add keeps minute points in memory until the next Flush.
func (s *Store) Add(points ...Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, points...)
}

// Flush writes the points Add kept, in one transaction. On failure they stay
// in memory for the next attempt.
func (s *Store) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	name, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO series (name) VALUES (?)`)
	if err != nil {
		return err
	}
	insert, err := tx.PrepareContext(ctx, `
INSERT OR REPLACE INTO points (series_id, resolution, time, average, peak)
VALUES ((SELECT id FROM series WHERE name = ?), ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, p := range s.pending {
		if _, err := name.ExecContext(ctx, p.Series); err != nil {
			return err
		}
		if _, err := insert.ExecContext(ctx, p.Series, Minute, p.Time.Unix(), p.Average, p.Peak); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.pending = s.pending[:0]
	return nil
}

// Maintain rolls finished periods up the ladder, deletes what has expired and
// gives the freed pages back to the filesystem.
func (s *Store) Maintain(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i := 1; i < len(ladder); i++ {
		if err := rollup(ctx, tx, ladder[i-1].res, ladder[i].res, now.Add(-ladder[i-1].keep), now); err != nil {
			return err
		}
	}
	for _, rung := range ladder {
		if _, err := tx.ExecContext(ctx, `DELETE FROM points WHERE resolution = ? AND time < ?`,
			rung.res, now.Add(-rung.keep).Unix()); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE time < ?`, now.Add(-eventsKeep).Unix()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`)
	return err
}

// rollup writes each finished target period whose source points are all still
// kept. A period that began before the source's window was rolled while it was
// whole, and is left as it is.
func rollup(ctx context.Context, tx *sql.Tx, from, to Resolution, kept, now time.Time) error {
	step := int64(to)
	start := (kept.Unix() + step - 1) / step * step
	end := now.Unix() / step * step
	if start >= end {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO points (series_id, resolution, time, average, peak)
SELECT series_id, ?, time / ? * ?, avg(average), max(peak)
FROM points
WHERE resolution = ? AND time >= ? AND time < ?
GROUP BY series_id, time / ?`,
		to, step, step, from, start, end, step)
	return err
}

// Close writes what is still in memory and closes the file.
func (s *Store) Close() error {
	return errors.Join(s.Flush(context.Background()), s.db.Close())
}

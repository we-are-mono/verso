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
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const driver = "sqlite"

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

	hmu      sync.Mutex
	health   Health
	onHealth func(was, now Health)
	failed   bool // Fail: no write here saves anything
}

// Open opens or creates the history file at path, readable by the shell's
// account alone, or keeps the history in memory for Memory. The directory must
// exist: creating it is root's, and an unwritable one is the caller's to report.
func Open(path string) (*Store, error) {
	db, err := connect(path)
	if err != nil {
		return nil, err
	}
	if err := shape(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, health: Health{OK: true, Since: time.Now()}}, nil
}

// connect opens one connection: the shell is the only writer, and one
// connection keeps the pragmas on every statement. auto_vacuum comes first:
// SQLite takes it only on a file with nothing written yet, and switching to
// WAL writes the file's header.
func connect(path string) (*sql.DB, error) {
	if path != Memory {
		if err := keepPrivate(path, sharedDirectory); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open(driver, "file:"+uriPath.Replace(path)+
		"?_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
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

// uriPath spells a path as the path of the file: URI SQLite opens, where ?
// starts the parameters, # a fragment and %xx stands for a byte, so a
// directory named with any of them holds the file rather than one its prefix
// or decoding names.
var uriPath = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")

// Memory is the path Open takes for a history kept in memory, which has no
// file.
const Memory = ":memory:"

// keepPrivate makes the file at path, creating it empty if it is absent, the
// WAL files beside it and its directory the shell's account's alone: it holds
// copies of configs only root may read, and the umask procd's jail runs the
// shell under leaves what is made readable to every account. SQLite gives the
// WAL files it makes the file's own mode; ones an earlier start left, and a
// directory root made readable, are tightened here. A directory that shared
// reports as other daemons' too keeps its mode; only the files are guarded.
func keepPrivate(path string, shared func(dir string) (bool, error)) error {
	dir := filepath.Dir(path)
	isShared, err := shared(dir)
	if err != nil {
		return err
	}
	if !isShared {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	err = f.Chmod(0o600)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	for _, side := range []string{path + "-wal", path + "-shm"} {
		if err := os.Chmod(side, 0o600); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// sharedDirectory reports whether dir is one other daemons keep their files in
// too, which an operator may name for history: /data itself, or a mount, a
// stick's root say, whose device differs from its parent's.
func sharedDirectory(dir string) (bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	if dir == "/data" {
		return true, nil
	}
	var here, above syscall.Stat_t
	if err := syscall.Stat(dir, &here); err != nil {
		return false, &fs.PathError{Op: "stat", Path: dir, Err: err}
	}
	if err := syscall.Stat(filepath.Dir(dir), &above); err != nil {
		return false, &fs.PathError{Op: "stat", Path: filepath.Dir(dir), Err: err}
	}
	return here.Dev != above.Dev, nil
}

// Add keeps minute points in memory until the next Flush.
func (s *Store) Add(points ...Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, points...)
}

// Flush writes the points Add kept, in one transaction. On failure they stay
// in memory for the next attempt; either way the health is noted.
func (s *Store) Flush(ctx context.Context) error {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return nil
	}
	err := s.flush(ctx)
	s.mu.Unlock()
	s.note(err, time.Now())
	return err
}

func (s *Store) flush(ctx context.Context) error {
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
// gives the freed pages back to the filesystem, noting the health.
func (s *Store) Maintain(ctx context.Context, now time.Time) error {
	err := s.maintain(ctx, now)
	s.note(err, time.Now())
	return err
}

func (s *Store) maintain(ctx context.Context, now time.Time) error {
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM alerts WHERE created < ?`, now.Add(-alertsKeep).Unix()); err != nil {
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

// Points reads one series at one resolution, oldest first, in [from, to).
// Minutes still in memory are included, so the newest quarter of an hour is
// not missing from a chart.
func (s *Store) Points(ctx context.Context, series string, res Resolution, from, to time.Time) ([]Point, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT time, average, peak FROM points
WHERE series_id = (SELECT id FROM series WHERE name = ?) AND resolution = ? AND time >= ? AND time < ?
ORDER BY time`,
		series, res, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Point
	for rows.Next() {
		p := Point{Series: series}
		var at int64
		if err := rows.Scan(&at, &p.Average, &p.Peak); err != nil {
			return nil, err
		}
		p.Time = time.Unix(at, 0)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == Minute {
		s.mu.Lock()
		for _, p := range s.pending {
			if p.Series == series && !p.Time.Before(from) && p.Time.Before(to) {
				out = append(out, p)
			}
		}
		s.mu.Unlock()
	}
	return out, nil
}

// Slots lays points out on n periods of res from start, each slot holding
// value(point) or NaN where nothing was recorded, so a chart's x is time.
func Slots(points []Point, value func(Point) float64, start time.Time, res Resolution, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for _, p := range points {
		if i := int(p.Time.Sub(start) / (time.Duration(res) * time.Second)); i >= 0 && i < n {
			out[i] = value(p)
		}
	}
	return out
}

// Record writes an event at once: events are rare, and the one that explains
// a power cut must not be lost with it.
func (s *Store) Record(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (time, kind, subject, detail) VALUES (?, ?, ?, ?)`,
		e.Time.Unix(), e.Kind, e.Subject, e.Detail)
	s.note(err, time.Now())
	return err
}

// RecordOnce records e, which root announced and the shell took from the
// spool under the name spool (ADR-028), unless an event of its time and kind
// already carries that spool name: a shell that stopped after recording an
// announcement and before letting it go records it once. id is the new
// event's number, 0 when it was recorded already.
func (s *Store) RecordOnce(ctx context.Context, e Event, spool string) (id int64, err error) {
	defer func() { s.note(err, time.Now()) }()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var seen int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE time = ? AND kind = ? AND json_valid(detail) AND json_extract(detail, '$.spool') = ?`,
		e.Time.Unix(), e.Kind, spool).Scan(&seen)
	if err != nil || seen > 0 {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO events (time, kind, subject, detail) VALUES (?, ?, ?, ?)`,
		e.Time.Unix(), e.Kind, e.Subject, e.Detail)
	if err != nil {
		return 0, err
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Events reads the events in [from, to), newest first.
func (s *Store) Events(ctx context.Context, from, to time.Time) ([]Event, error) {
	return s.events(ctx, `WHERE time >= ? AND time < ? ORDER BY time DESC, rowid DESC`, from.Unix(), to.Unix())
}

// Latest is the newest event of one kind; found is false when there is none.
func (s *Store) Latest(ctx context.Context, kind string) (e Event, found bool, err error) {
	events, err := s.events(ctx, `WHERE kind = ? ORDER BY time DESC, rowid DESC LIMIT 1`, kind)
	if err != nil || len(events) == 0 {
		return Event{}, false, err
	}
	return events[0], true, nil
}

func (s *Store) events(ctx context.Context, where string, args ...any) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT time, kind, subject, detail FROM events `+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Event
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&at, &e.Kind, &e.Subject, &e.Detail); err != nil {
			return nil, err
		}
		e.Time = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Close writes what is still in memory and closes the file.
func (s *Store) Close() error {
	return errors.Join(s.Flush(context.Background()), s.db.Close())
}

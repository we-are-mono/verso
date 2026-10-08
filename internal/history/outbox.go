// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"time"
)

// outbox is the alerts waiting to be sent and the record of each attempt
// (ADR-020). Created on every open if absent, so a history kept before it
// gains it without losing its points.
const outbox = `
CREATE TABLE IF NOT EXISTS alerts (
	id       INTEGER PRIMARY KEY,
	created  INTEGER NOT NULL,
	title    TEXT    NOT NULL,
	body     TEXT    NOT NULL DEFAULT '',
	priority TEXT    NOT NULL DEFAULT '',
	state    TEXT    NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	next_try INTEGER NOT NULL DEFAULT 0,
	last_try INTEGER NOT NULL DEFAULT 0,
	result   TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS alerts_due ON alerts (state, next_try);`

// alertsKeep is how long the outbox remembers an alert.
const alertsKeep = 30 * 24 * time.Hour

// Alert is one message for the operator. State is "pending" until it is
// "sent", "failed" (given up) or "cancelled"; Result is the last attempt's
// answer: an HTTP status, or why there was none. Hold, when set, is when it is
// first tried; otherwise at once.
type Alert struct {
	ID       int64
	Created  time.Time
	Hold     time.Time
	Title    string
	Body     string
	Priority string
	State    string
	Attempts int
	LastTry  time.Time
	Result   string
}

// Enqueue keeps an alert to be sent from its hold, or at once.
func (s *Store) Enqueue(ctx context.Context, a Alert) (int64, error) {
	first := a.Created
	if !a.Hold.IsZero() {
		first = a.Hold
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO alerts (created, title, body, priority, next_try) VALUES (?, ?, ?, ?, ?)`,
		a.Created.Unix(), a.Title, a.Body, a.Priority, first.Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Due is the pending alerts whose next try has come, oldest first.
func (s *Store) Due(ctx context.Context, now time.Time) ([]Alert, error) {
	return s.alerts(ctx, `WHERE state = 'pending' AND next_try <= ? ORDER BY id`, now.Unix())
}

// LastAlert is the alert most recently tried; found is false before any was.
func (s *Store) LastAlert(ctx context.Context) (a Alert, found bool, err error) {
	tried, err := s.alerts(ctx, `WHERE attempts > 0 ORDER BY last_try DESC, id DESC LIMIT 1`)
	if err != nil || len(tried) == 0 {
		return Alert{}, false, err
	}
	return tried[0], true, nil
}

func (s *Store) alerts(ctx context.Context, where string, args ...any) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, created, title, body, priority, state, attempts, last_try, result FROM alerts `+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Alert
	for rows.Next() {
		var a Alert
		var created, lastTry int64
		if err := rows.Scan(&a.ID, &created, &a.Title, &a.Body, &a.Priority, &a.State, &a.Attempts, &lastTry, &a.Result); err != nil {
			return nil, err
		}
		a.Created, a.LastTry = time.Unix(created, 0), time.Unix(lastTry, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Attempted records an attempt made at: the alert's new state, the answer,
// and when to try again while it stays pending.
func (s *Store) Attempted(ctx context.Context, id int64, at time.Time, state, result string, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE alerts SET state = ?, result = ?, attempts = attempts + 1, last_try = ?, next_try = ? WHERE id = ?`,
		state, result, at.Unix(), next.Unix(), id)
	return err
}

// Rewrite changes what a pending alert says; ok is false once it has gone.
func (s *Store) Rewrite(ctx context.Context, id int64, title, body string) (ok bool, err error) {
	return s.whilePending(ctx, `UPDATE alerts SET title = ?, body = ? WHERE id = ? AND state = 'pending'`, title, body, id)
}

// Cancel withdraws a pending alert; ok is false once it has gone.
func (s *Store) Cancel(ctx context.Context, id int64) (ok bool, err error) {
	return s.whilePending(ctx, `UPDATE alerts SET state = 'cancelled' WHERE id = ? AND state = 'pending'`, id)
}

func (s *Store) whilePending(ctx context.Context, query string, args ...any) (bool, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// EventsAfter is the events recorded after the one numbered after, oldest
// first, and the number of the last of them (after itself when there are
// none): a reader that keeps it is handed each event once.
func (s *Store) EventsAfter(ctx context.Context, after int64) ([]Event, int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rowid, time, kind, subject, detail FROM events WHERE rowid > ? ORDER BY rowid`, after)
	if err != nil {
		return nil, after, err
	}
	defer func() { _ = rows.Close() }()
	var out []Event
	last := after
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&last, &at, &e.Kind, &e.Subject, &e.Detail); err != nil {
			return nil, after, err
		}
		e.Time = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, last, rows.Err()
}

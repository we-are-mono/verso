// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"time"
)

// Record writes an event at once: events are rare, and the one that explains
// a power cut must not be lost with it.
func (s *Store) Record(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (time, kind, subject, detail) VALUES (?, ?, ?, ?)`,
		e.Time.Unix(), e.Kind, e.Subject, e.Detail)
	return err
}

// Events reads the events in [from, to), newest first.
func (s *Store) Events(ctx context.Context, from, to time.Time) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT time, kind, subject, detail FROM events
WHERE time >= ? AND time < ?
ORDER BY time DESC, rowid DESC`,
		from.Unix(), to.Unix())
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

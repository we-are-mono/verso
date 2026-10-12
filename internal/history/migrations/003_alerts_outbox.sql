-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- The alerts waiting to be sent and the record of each attempt (ADR-020).
-- Only if absent: shells from before the migrations created it on every open,
-- so a file at version 2 may hold it already, with alerts in it.

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
CREATE INDEX IF NOT EXISTS alerts_due ON alerts (state, next_try);

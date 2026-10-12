-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Measurements keyed by the series' name, and the events daemons announced.

CREATE TABLE points (
	series     TEXT    NOT NULL,
	resolution INTEGER NOT NULL,
	time       INTEGER NOT NULL,
	average    REAL    NOT NULL,
	peak       REAL    NOT NULL,
	PRIMARY KEY (series, resolution, time)
) WITHOUT ROWID;

CREATE TABLE events (
	time    INTEGER NOT NULL,
	kind    TEXT    NOT NULL,
	subject TEXT    NOT NULL,
	detail  TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX events_time ON events (time);

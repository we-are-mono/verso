-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- A series' name stored once, its points carrying its id.

CREATE TABLE series (
	id   INTEGER PRIMARY KEY,
	name TEXT    NOT NULL UNIQUE
);
INSERT INTO series (name) SELECT DISTINCT series FROM points ORDER BY series;

CREATE TABLE points_by_id (
	series_id  INTEGER NOT NULL REFERENCES series (id),
	resolution INTEGER NOT NULL,
	time       INTEGER NOT NULL,
	average    REAL    NOT NULL,
	peak       REAL    NOT NULL,
	PRIMARY KEY (series_id, resolution, time)
) WITHOUT ROWID;
INSERT INTO points_by_id (series_id, resolution, time, average, peak)
	SELECT series.id, points.resolution, points.time, points.average, points.peak
	FROM points JOIN series ON series.name = points.series;

DROP TABLE points;
ALTER TABLE points_by_id RENAME TO points;

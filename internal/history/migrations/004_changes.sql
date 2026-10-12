-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Config history (ADR-021): each apply, the rows it read as in the review
-- drawer, and the uci changes under each row.

-- One apply, or one edit written straight to the config. status is applied,
-- rolledback or undone. user and address are the rpcd session's user and the
-- browser's address, empty for a direct edit. relation is '', undo, return or
-- again, and related the change it names.
CREATE TABLE changes (
	id        INTEGER PRIMARY KEY,
	time      INTEGER NOT NULL,
	status    TEXT    NOT NULL,
	user      TEXT    NOT NULL DEFAULT '',
	address   TEXT    NOT NULL DEFAULT '',
	note      TEXT    NOT NULL DEFAULT '',
	relation  TEXT    NOT NULL DEFAULT '',
	related   INTEGER REFERENCES changes (id),
	undone_by INTEGER REFERENCES changes (id)
);
CREATE INDEX changes_time ON changes (time);

-- A row as the review drawer read it: the plugin's words for it (or the raw
-- uci line, verbatim), what happened in a word, and the object it is about.
CREATE TABLE change_items (
	change_id    INTEGER NOT NULL REFERENCES changes (id),
	position     INTEGER NOT NULL,
	page         TEXT    NOT NULL,
	plugin       TEXT    NOT NULL DEFAULT '',
	verb         TEXT    NOT NULL,
	words        TEXT    NOT NULL,
	verbatim     INTEGER NOT NULL DEFAULT 0,
	subject_kind TEXT    NOT NULL DEFAULT '',
	subject_name TEXT    NOT NULL DEFAULT '',
	PRIMARY KEY (change_id, position)
) WITHOUT ROWID;

-- One uci change under a row. value is what the option became and old what
-- it replaced; NULL is no value, which an empty string is not. A secret keeps
-- neither.
CREATE TABLE change_ops (
	change_id INTEGER NOT NULL REFERENCES changes (id),
	item      INTEGER NOT NULL,
	position  INTEGER NOT NULL,
	config    TEXT    NOT NULL,
	section   TEXT    NOT NULL,
	op        TEXT    NOT NULL,
	option    TEXT    NOT NULL DEFAULT '',
	value     TEXT,
	old       TEXT,
	secret    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (change_id, item, position)
) WITHOUT ROWID;
CREATE INDEX change_ops_section ON change_ops (config, section);

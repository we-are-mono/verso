-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Config history's starting point (ADR-021): each config as it stood before
-- the oldest entry kept, as verso-rpcd spools it, and since when. It is the
-- config as first read, and each entry folded out of history moves it on.
CREATE TABLE config_start (
	config   TEXT    PRIMARY KEY,
	sections TEXT    NOT NULL,
	time     INTEGER NOT NULL
) WITHOUT ROWID;

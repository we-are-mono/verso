-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Each config as config history last read it (ADR-021): its committed
-- sections as verso-rpcd spools them, secrets as fingerprints, and when it
-- was written. An edit made outside Verso is told by what differs from it.
CREATE TABLE config_state (
	config   TEXT    PRIMARY KEY,
	sections TEXT    NOT NULL,
	time     INTEGER NOT NULL
) WITHOUT ROWID;

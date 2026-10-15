-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- A scrub of secrets owes the file a rewrite (ADR-021): its one row is
-- written with the rows the scrub writes, and taken out only once VACUUM and
-- an emptied log leave no copy of what they held. While it is there, every
-- scrub rewrites the file, though it finds no row to write.
CREATE TABLE rewrite_owed (
	id INTEGER PRIMARY KEY CHECK (id = 1)
);

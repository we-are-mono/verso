-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- The arguments of a change's cause, a JSON list in the order its sentence's
-- %s takes them: the package a package operation names (ADR-021). Kept apart
-- from the sentence so the sentence translates and each argument stays as
-- written. Empty for a cause with none.
ALTER TABLE changes ADD COLUMN cause_args TEXT NOT NULL DEFAULT '';

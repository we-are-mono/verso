-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Whether the values each change replaced were read before the apply (ADR-021).
-- An entry recorded without them has nothing to be undone to.
ALTER TABLE changes ADD COLUMN old_read INTEGER NOT NULL DEFAULT 0;

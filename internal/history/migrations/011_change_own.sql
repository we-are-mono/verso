-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- Whether the router committed the change itself (ADR-021). A plugin built
-- before causes marks its commits with none, so the cause alone cannot say
-- whether a change with no operator was the router's or made outside Verso.
ALTER TABLE changes ADD COLUMN own INTEGER NOT NULL DEFAULT 0;

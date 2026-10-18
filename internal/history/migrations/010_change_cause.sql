-- SPDX-License-Identifier: GPL-2.0-only
-- SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

-- What made a change the router committed itself, in English, and the plugin
-- whose catalog words it, empty for the shell's own words (ADR-021). A change
-- an operator applied, or one made outside Verso, has neither.
ALTER TABLE changes ADD COLUMN cause TEXT NOT NULL DEFAULT '';
ALTER TABLE changes ADD COLUMN cause_plugin TEXT NOT NULL DEFAULT '';

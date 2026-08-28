// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package profiles embeds the per-board hardware profiles so the shell ships
// them in its binary. A profile is chosen by the board's board_name (the folder
// name); see README.md for the format. Contributors add a board by dropping a
// folder with a profile.json — the glob picks it up, no code change.
package profiles

import "embed"

// FS holds every board's profile.json, addressed as "<board_name>/profile.json".
//
//go:embed */profile.json
var FS embed.FS

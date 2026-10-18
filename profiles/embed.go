// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package profiles embeds the per-board hardware profiles so the shell ships
// them in its binary. A profile is chosen by the board's board_name (the folder
// name); see README.md for the format. Contributors add a board by dropping a
// folder with a profile.json — the glob picks it up, no code change.
package profiles

import "embed"

// FS holds every board's profile.json, its optional panel artwork and its
// optional photograph, addressed as "<board_name>/profile.json",
// "<board_name>/back.svg" (or front.svg) and "<board_name>/board.png". The
// panel art is first-party content shipped in the binary, so the Overview
// renders it as trusted core content; the photograph stands beside the facts
// of the box on the Hardware page.
//
//go:embed */profile.json */*.svg */board.png
var FS embed.FS

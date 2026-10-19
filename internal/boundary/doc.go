// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package boundary holds the test that keeps the shell from naming a plugin:
// core may know OpenWrt and its own closed vocabularies, never a plugin's id,
// address or nav label. Its test reads the shell's Go by type and the
// helper's Rust by its includes, and fails on the first place that does.
package boundary

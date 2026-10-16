// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"log"
	"testing"
)

// TestTheShellsLogLinesCarryOneTimestamp: procd hands the shell's output to
// logd, which stamps every line, so the shell's own lines carry no second
// stamp of their own.
func TestTheShellsLogLinesCarryOneTimestamp(t *testing.T) {
	defer log.SetFlags(log.Flags())
	log.SetFlags(log.LstdFlags)
	info := procdLoggers()
	if log.Flags() != 0 || info.Flags() != 0 {
		t.Errorf("flags = %d (info %d), want no prefix of their own", log.Flags(), info.Flags())
	}
}

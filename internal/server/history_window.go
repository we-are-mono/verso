// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "github.com/we-are-mono/verso/internal/history"

// historyWindow is one past window a history graph offers (ADR-019): its value
// on the wire, its words in the dropdown, and its periods at one resolution.
type historyWindow struct {
	Value, Label string
	Res          history.Resolution
	N            int
}

// historyWindows are the past windows every history graph offers, in the
// dropdown's order: a day of minutes, a week of quarters, a month of hours, a
// year of days.
var historyWindows = []historyWindow{
	{"day", "Last 24 hours", history.Minute, 24 * 60},
	{"week", "Last 7 days", history.Quarter, 7 * 24 * 4},
	{"month", "Last 30 days", history.Hour, 30 * 24},
	{"year", "Last year", history.Day, 365},
}

// historyWindowOf is the past window value names.
func historyWindowOf(value string) (historyWindow, bool) {
	for _, w := range historyWindows {
		if w.Value == value {
			return w, true
		}
	}
	return historyWindow{}, false
}

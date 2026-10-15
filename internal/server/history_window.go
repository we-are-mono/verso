// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// historyWindow is one past window a history graph offers (ADR-019): its value
// on the wire, its words in the dropdown, and its periods at one resolution.
type historyWindow struct {
	Value, Label string
	Res          history.Resolution
	N            int
}

// Span is how far back the window reaches.
func (w historyWindow) Span() time.Duration {
	return time.Duration(w.N) * w.step()
}

// step is one period of the window's resolution.
func (w historyWindow) step() time.Duration { return time.Duration(w.Res) * time.Second }

// bounds are the window's periods as of now: from the start of the first to
// the end of the one now falls in.
func (w historyWindow) bounds(now time.Time) (start, end time.Time) {
	start = now.Truncate(w.step()).Add(-time.Duration(w.N-1) * w.step())
	return start, start.Add(w.Span())
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

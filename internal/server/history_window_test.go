// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// TestHistoryWindowsLayEachWindowOut: each past window a history graph offers
// is periods of one resolution (ADR-019), in the order the dropdown lists
// them, spanning what its label says.
func TestHistoryWindowsLayEachWindowOut(t *testing.T) {
	want := []struct {
		value, label string
		res          history.Resolution
		n            int
		span         time.Duration
	}{
		{"day", "Last 24 hours", history.Minute, 24 * 60, 24 * time.Hour},
		{"week", "Last 7 days", history.Quarter, 7 * 24 * 4, 7 * 24 * time.Hour},
		{"month", "Last 30 days", history.Hour, 30 * 24, 30 * 24 * time.Hour},
		{"year", "Last year", history.Day, 365, 365 * 24 * time.Hour},
	}
	if len(historyWindows) != len(want) {
		t.Fatalf("historyWindows has %d windows, want %d", len(historyWindows), len(want))
	}
	for i, w := range want {
		got := historyWindows[i]
		span := time.Duration(got.N) * time.Duration(got.Res) * time.Second
		if got.Value != w.value || got.Label != w.label || got.Res != w.res || got.N != w.n || span != w.span {
			t.Errorf("window %d = %+v (span %v), want %+v", i, got, span, w)
		}
		if found, ok := historyWindowOf(w.value); !ok || found != got {
			t.Errorf("historyWindowOf(%q) = %+v, %v", w.value, found, ok)
		}
	}
	if _, ok := historyWindowOf("minute"); ok {
		t.Error("the live minute is no past window")
	}
}

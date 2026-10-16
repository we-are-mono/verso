// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package units

import (
	"encoding/json"
	"os"
	"testing"
)

type durationCase struct {
	Seconds  int64  `json:"seconds"`
	Duration string `json:"duration"`
	Since    string `json:"since"`
}

func readDurations(t *testing.T) []durationCase {
	t.Helper()
	data, err := os.ReadFile("testdata/durations.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Durations []durationCase `json:"durations"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f.Durations
}

func TestDurationsReadInTheirTwoLargestUnits(t *testing.T) {
	for _, tc := range readDurations(t) {
		if got := Duration(tc.Seconds); got != tc.Duration {
			t.Errorf("Duration(%d) = %q, want %q", tc.Seconds, got, tc.Duration)
		}
		if got := Since(tc.Seconds); got != tc.Since {
			t.Errorf("Since(%d) = %q, want %q", tc.Seconds, got, tc.Since)
		}
	}
}

// An axis counts in no unit larger than its step reaches, so a day's quarters
// read in hours and a week's in days and hours.
func TestAnAxisCountsInItsStepsUnit(t *testing.T) {
	for _, tc := range []struct {
		seconds, step int64
		want          string
	}{
		{86400, 21600, "24 h"},      // a day in quarters of 6 h
		{64800, 21600, "18 h"},      // its first inner quarter
		{604800, 151200, "7 d"},     // a week in quarters of 1 d 18 h
		{453600, 151200, "5 d 6 h"}, // its first inner quarter
		{60, 15, "60 s"},            // a minute in quarters of 15 s
		{2592000, 648000, "30 d"},   // a month
		{1944000, 648000, "22 d 12 h"},
	} {
		if got := DurationWithin(tc.seconds, tc.step); got != tc.want {
			t.Errorf("DurationWithin(%d, %d) = %q, want %q", tc.seconds, tc.step, got, tc.want)
		}
	}
}

// A time ago is translated as a whole pattern, so a language puts its word for
// ago where it belongs.
func TestAgoIsTheLanguagesPattern(t *testing.T) {
	if got := For(nil).Ago(8040); got != "2 h ago" {
		t.Errorf("English ago = %q", got)
	}
	slovenian := func(s string) string {
		if s == "%s ago" {
			return "pred %s"
		}
		return s
	}
	if got := For(slovenian).Ago(754); got != "pred 12 min" {
		t.Errorf("Slovenian ago = %q", got)
	}
	for _, seconds := range []int64{-3, 0, 59} {
		if got := For(nil).Ago(seconds); got != "just now" {
			t.Errorf("Ago(%d) = %q, want under a minute said in words", seconds, got)
		}
	}
}

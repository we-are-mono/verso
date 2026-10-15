// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"strings"
	"testing"
	"time"
)

// TestParseIgnoresReadsEntriesAndSkipsComments: one entry per line, the reason
// is the rest of the line, and comment and blank lines carry nothing.
func TestParseIgnoresReadsEntriesAndSkipsComments(t *testing.T) {
	list := `# Triaged advisories.

GO-2026-0001 2026-12-31 the vulnerable parser is never fed untrusted input
  RUSTSEC-2026-0002   2027-01-15   build-time only   # spaced out
`
	got, err := ParseIgnores(strings.NewReader(list), today)
	if err != nil {
		t.Fatalf("ParseIgnores: %v", err)
	}
	want := []Ignore{
		{ID: "GO-2026-0001", Until: day(2026, 12, 31), Reason: "the vulnerable parser is never fed untrusted input", Line: 3},
		{ID: "RUSTSEC-2026-0002", Until: day(2027, 1, 15), Reason: "build-time only   # spaced out", Line: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParseIgnoresRejectsIncompleteEntries: an ignore without a date or a
// reason is not a triage, so the list refuses it and names its line.
func TestParseIgnoresRejectsIncompleteEntries(t *testing.T) {
	for name, line := range map[string]string{
		"no date":     "GO-2026-0001",
		"no reason":   "GO-2026-0001 2026-12-31",
		"bad date":    "GO-2026-0001 31.12.2026 a reason",
		"date as id":  "2026-12-31 GO-2026-0001 a reason",
		"duplicate":   "GO-2026-0001 2026-12-31 a reason\nGO-2026-0001 2026-12-31 again",
		"lowercase":   "go-2026-0001 2026-12-31 a reason",
		"id with url": "https://pkg.go.dev/vuln/GO-2026-0001 2026-12-31 a reason",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseIgnores(strings.NewReader(line), today)
			if err == nil {
				t.Fatalf("ParseIgnores(%q) accepted an incomplete entry", line)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Errorf("error %q does not name the line", err)
			}
		})
	}
}

// TestParseIgnoresCapsTheLapseDate: a triage lapses at most MaxTriageDays
// after the day the list is read, so no entry silences an advisory for good.
// The last day within reach is accepted, the day after it refused.
func TestParseIgnoresCapsTheLapseDate(t *testing.T) {
	last := today.AddDate(0, 0, MaxTriageDays).Format(dateLayout)
	if _, err := ParseIgnores(strings.NewReader("GO-2026-0001 "+last+" a reason"), today); err != nil {
		t.Fatalf("an entry lapsing %d days out was refused: %v", MaxTriageDays, err)
	}
	for name, date := range map[string]string{
		"one day too far": today.AddDate(0, 0, MaxTriageDays+1).Format(dateLayout),
		"years out":       "2099-12-31",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseIgnores(strings.NewReader("GO-2026-0001 "+date+" a reason"), today)
			if err == nil {
				t.Fatalf("an entry lapsing %s was accepted", date)
			}
			if !strings.Contains(err.Error(), "line 1") {
				t.Errorf("error %q does not name the line", err)
			}
		})
	}
}

// TestIgnoreActiveThroughItsDate: an entry holds through the whole of its
// date and lapses the day after, so a triage is revisited on schedule.
func TestIgnoreActiveThroughItsDate(t *testing.T) {
	ig := Ignore{ID: "GO-2026-0001", Until: day(2026, 12, 31), Reason: "r"}
	if !ig.Active(time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)) {
		t.Error("entry lapsed on its own date")
	}
	if ig.Active(time.Date(2027, 1, 1, 0, 0, 1, 0, time.UTC)) {
		t.Error("entry still active the day after its date")
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

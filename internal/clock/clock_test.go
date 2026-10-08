// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package clock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestZoneFollowsThePOSIXRule: OpenWrt's /etc/TZ is a POSIX rule, its daylight
// saving included, in either hemisphere.
func TestZoneFollowsThePOSIXRule(t *testing.T) {
	for _, c := range []struct {
		rule       string
		at         time.Time
		wantClock  string
		wantAbbrev string
	}{
		{"CET-1CEST,M3.5.0,M10.5.0/3", time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "13:00", "CET"},
		{"CET-1CEST,M3.5.0,M10.5.0/3", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "14:00", "CEST"},
		// Summer time begins at 01:00 UTC on the last Sunday of March.
		{"CET-1CEST,M3.5.0,M10.5.0/3", time.Date(2026, 3, 29, 0, 59, 0, 0, time.UTC), "01:59", "CET"},
		{"CET-1CEST,M3.5.0,M10.5.0/3", time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC), "03:00", "CEST"},
		{"AEST-10AEDT,M10.1.0,M4.1.0/3", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), "11:00", "AEDT"},
		{"EST5EDT,M3.2.0,M11.1.0", time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "07:00", "EST"},
		{"UTC0", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "12:00", "UTC"},
		// The bare name some images write reads as the zero offset it means.
		{"UTC", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "12:00", "UTC"},
		{"<+0530>-5:30", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "17:30", "+0530"},
	} {
		zone, err := Location(c.rule)
		if err != nil {
			t.Fatalf("Location(%q): %v", c.rule, err)
		}
		local := c.at.In(zone)
		if abbrev, _ := local.Zone(); local.Format("15:04") != c.wantClock || abbrev != c.wantAbbrev {
			t.Errorf("%q at %v = %s %s, want %s %s", c.rule, c.at, local.Format("15:04"), abbrev, c.wantClock, c.wantAbbrev)
		}
	}
}

// TestLocationRefusesWhatIsNoRule.
func TestLocationRefusesWhatIsNoRule(t *testing.T) {
	for _, bad := range []string{"", "  ", "CET-1CEST,M3.5.0", "not a zone\n"} {
		if _, err := Location(bad); err == nil {
			t.Errorf("Location(%q) succeeded", bad)
		}
	}
}

// TestZoneRereadsTheFileWhenItChanges: an operator who changes the time zone
// sees it without restarting anything; a missing file is the host's own zone.
func TestZoneRereadsTheFileWhenItChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TZ")
	z := &zone{path: path, every: 0}
	if got := z.get(); got != time.Local {
		t.Fatalf("no file = %v, want the host's zone", got)
	}
	if err := os.WriteFile(path, []byte("CET-1CEST,M3.5.0,M10.5.0/3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if abbrev, _ := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(z.get()).Zone(); abbrev != "CET" {
		t.Fatalf("after writing CET: %s", abbrev)
	}
	if err := os.WriteFile(path, []byte("EST5EDT,M3.2.0,M11.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if abbrev, _ := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(z.get()).Zone(); abbrev != "EST" {
		t.Fatalf("after writing EST: %s", abbrev)
	}
	if err := os.WriteFile(path, []byte("not a zone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if abbrev, _ := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(z.get()).Zone(); abbrev != "EST" {
		t.Fatalf("a broken file replaced the last good zone: %s", abbrev)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"
)

// A session is where it signed in from and when it was last seen: the browser
// reads under the address, and when it started under when it was last seen,
// so a full IPv6 address and a way to revoke fit the form's measure. Revoking
// is an icon on the row that asks first.
func TestSessionsFoldIntoSourceAndLastSeen(t *testing.T) {
	table := accessSessionsTable([]accessSession{
		{ID: "a", Address: "10.0.0.232", Browser: "Firefox · Linux", Since: "since today, 16:43", LastActive: "Now", Current: true},
		{ID: "b", Address: "fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b", Browser: "Safari · iPhone", Since: "since 22 Sep, 09:12", LastActive: "3 min ago"},
	}, identity)

	var labels, kinds []string
	for _, c := range table.Columns {
		labels, kinds = append(labels, c.Label), append(kinds, c.Kind)
	}
	if len(labels) != 3 || labels[0] != "Source" || labels[1] != "Last seen" || labels[2] != "" ||
		kinds[0] != "reference" || kinds[1] != "text" || kinds[2] != "actions" {
		t.Fatalf("columns = %v %v, want Source, Last seen and the row's act", labels, kinds)
	}

	current, other := table.Rows[0].Cells, table.Rows[1].Cells
	if src := other[0]; src.Text != "fd42:7ea:aa00::1a2b:3c4d:5e6f:7a8b" || src.Detail != "Safari · iPhone" || src.Sub != "" {
		t.Errorf("source = %+v, want the address over its browser", src)
	}
	if seen := other[1]; seen.Text != "3 min ago" || seen.Detail != "since 22 Sep, 09:12" {
		t.Errorf("last seen = %+v, want the age over when it started", seen)
	}
	acts := other[2].Actions
	if len(acts) != 1 || acts[0].Icon != "log-out" || acts[0].Title != "Revoke session" ||
		acts[0].Name != "_action" || acts[0].Value != "end-session:b" || other[2].Confirm == "" {
		t.Errorf("revoke = %+v (confirm %q), want one icon act that asks first", acts, other[2].Confirm)
	}

	if current[0].Tag != "this browser" || len(current[2].Actions) != 0 {
		t.Errorf("this browser is marked and cannot revoke itself: %+v", current)
	}
}

// When a session started is said as "since", today by the clock alone and any
// other day by its date.
func TestSessionSince(t *testing.T) {
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		created time.Time
		want    string
	}{
		{time.Date(2026, 9, 23, 16, 43, 0, 0, time.Local), "since today, 16:43"},
		{time.Date(2026, 9, 22, 9, 12, 0, 0, time.Local), "since 22 Sep, 09:12"},
	} {
		if got := sessionSince(identity, tc.created, now); got != tc.want {
			t.Errorf("sessionSince(%v) = %q, want %q", tc.created, got, tc.want)
		}
	}
}

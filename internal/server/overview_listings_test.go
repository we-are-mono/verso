// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "testing"

func TestLeaseRemaining(t *testing.T) {
	const now = 1_000_000
	cases := []struct {
		name   string
		expiry int64
		want   string
	}{
		{"hours and minutes", now + 11*3600 + 12*60, "11h 12m"},
		{"minutes only", now + 30*60, "30m"},
		{"sub-minute rounds down", now + 59, "0m"},
		{"already expired", now - 5, "expired"},
		{"at expiry", now, "expired"},
		{"static (no expiry) is blank", -1, ""},
		{"pads minutes", now + 3600 + 5*60, "1h 05m"},
	}
	for _, c := range cases {
		if got := leaseRemaining(c.expiry, now); got != c.want {
			t.Errorf("%s: leaseRemaining(%d, %d) = %q, want %q", c.name, c.expiry, now, got, c.want)
		}
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "testing"

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{0, "0m"},
		{59, "0m"},
		{60, "1m"},
		{3600, "1h 0m"},
		{3661, "1h 1m"},
		{90000, "1d 1h 0m"},
	}
	for _, c := range cases {
		if got := formatUptime(c.sec); got != c.want {
			t.Errorf("formatUptime(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}

func TestFormatLoad(t *testing.T) {
	cases := []struct {
		raw  int64
		want string
	}{
		{0, "0.00"},
		{65536, "1.00"},
		{1566912, "23.91"},
	}
	for _, c := range cases {
		if got := formatLoad(c.raw); got != c.want {
			t.Errorf("formatLoad(%d) = %q, want %q", c.raw, got, c.want)
		}
	}
}

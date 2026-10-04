// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "testing"

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

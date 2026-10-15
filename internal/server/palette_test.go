// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "testing"

// The shell reads its own palette at startup, so what it writes outside the
// stylesheet (a takeover's browser bar colour) is the palette's value.
func TestTheServerCarriesThePalette(t *testing.T) {
	s := newServer(t, fakeBackend{})
	for name, want := range map[string]string{"canvas": "#ffffff", "ground": "#faf9f7"} {
		if got, ok := s.palette.Colour(name); !ok || got != want {
			t.Errorf("palette %q = %q, %v; want %q", name, got, ok, want)
		}
	}
}

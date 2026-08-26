// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderFieldChecks: the checks kind is membership in a known set — every
// box shares the field's name (posting multi-value, the list contract), the
// current values come back checked, and the rest don't.
func TestRenderFieldChecks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{
		Name: "network", Label: "Networks", Kind: "checks",
		Values: []string{"lan", "lan2"},
		Options: []Option{
			{Value: "lan", Label: "lan"}, {Value: "lan2", Label: "lan2"}, {Value: "guest", Label: "guest"},
		},
	})
	if strings.Count(got, `name="network"`) != 3 {
		t.Errorf("all boxes must share the field name:\n%s", got)
	}
	if strings.Count(got, " checked") != 2 {
		t.Errorf("exactly the current values should be checked:\n%s", got)
	}
	for _, want := range []string{`type="checkbox"`, ">Networks<", "guest"} {
		if !strings.Contains(got, want) {
			t.Errorf("checks field missing %q:\n%s", want, got)
		}
	}
}

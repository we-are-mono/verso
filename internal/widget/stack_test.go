// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestARowOfActsIsTwoCellsTall: an inline row stands its controls on the
// notebook's two cells, 3px over and under a 34px control. Acts are 28px, so a
// row of them pads 6px instead and steps its wrapped lines 12px apart: every
// line of acts is two cells too, and what follows stays on the grid.
func TestARowOfActsIsTwoCellsTall(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stack{Inline: true, Children: []Widget{
		&Link{Label: "Install a certificate", Href: "/x", Style: "act"},
		&Link{Label: "Download", Href: "/y", Style: "act"},
	}})
	for _, want := range []string{"py-0.75", "has-[.verso-act]:py-1.5", "has-[.verso-act]:gap-y-3"} {
		if !strings.Contains(got, want) {
			t.Errorf("an inline row is missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "verso-act ") != 2 {
		t.Errorf("each act carries the mark the row reads:\n%s", got)
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestARowOfActsIsAControlTall: an inline row is a 32px control's height, so
// the control's edges are the row's and land on the grid's lines, its wrapped
// lines 40px apart. Acts are 28px, so a row of them pads 2px and steps its
// wrapped lines 12px apart: every line of acts keeps the same 40px pitch, and
// what follows stays on the grid.
func TestARowOfActsIsAControlTall(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Stack{Inline: true, Children: []Widget{
		&Link{Label: "Install a certificate", Href: "/x", Style: "act"},
		&Link{Label: "Download", Href: "/y", Style: "act"},
	}})
	if strings.Contains(got, " py-1") {
		t.Errorf("an inline row adds no air over its controls:\n%s", got)
	}
	for _, want := range []string{"has-[.verso-act]:py-0.5", "has-[.verso-act]:gap-y-3"} {
		if !strings.Contains(got, want) {
			t.Errorf("an inline row is missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "verso-act ") != 2 {
		t.Errorf("each act carries the mark the row reads:\n%s", got)
	}
}

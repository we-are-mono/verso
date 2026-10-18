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
	if !strings.HasPrefix(got, `<div class="verso-cluster">`) {
		t.Errorf("an inline row is not a cluster:\n%s", got)
	}
	if rule := cssRule(blockCSS(t, "cluster"), ".verso-cluster"); strings.Contains(rule, "padding") {
		t.Errorf("an inline row adds no air over its controls:\n%s", rule)
	}
	declares(t, "cluster", `.verso-cluster:has([data-verso-size="act"])`,
		"row-gap: calc(var(--spacing) * 3)", "padding-block: calc(var(--spacing) * 0.5)")
	if strings.Count(got, `data-verso-size="act"`) != 2 {
		t.Errorf("each act carries the mark the row reads:\n%s", got)
	}
}

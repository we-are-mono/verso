// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderFilter: the lens renders its sentinel (pinned-state detection), the
// ground-coloured sticky dock, the search input with the plugin's placeholder,
// and the "/" hint. Behaviour lives in verso.js — the widget is declarative.
func TestRenderFilter(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Filter{Placeholder: "Filter — zone, port, IP, comment…"})
	for _, want := range []string{
		"verso-filter-dock", "sticky",
		"data-verso-filter",
		`placeholder="Filter — zone, port, IP, comment…"`,
		">/</kbd>",
		lucideIcons["search"],
	} {
		if !strings.Contains(got, want) {
			t.Errorf("filter missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(render(t, r, &Filter{}), `placeholder="Filter…"`) {
		t.Error("filter without a placeholder should fall back to a generic one")
	}
}

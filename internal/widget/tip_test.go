// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestEveryTipIsTheTipBlock: a copy's tip, an act's tip and a chip's
// explanation are one tooltip (blocks/tip.css): a control's tip is raised by
// its trigger's group, a sentence tip names its kind and is raised by its
// label.
func TestEveryTipIsTheTipBlock(t *testing.T) {
	r := newRenderer(t)
	copyable := &Properties{Items: []Property{{Label: "Address", Value: "192.0.2.7", Mono: true, Copy: true}}}
	explained := &Table{
		Columns: []TableColumn{{Label: "Via", Kind: "entity"}},
		Rows:    []TableRow{{ID: "r", Cells: []TableCell{{Chips: []TableChip{{Label: "lan", Title: "The local network"}}}}}},
	}
	for _, c := range []struct {
		name string
		w    Widget
		want string
	}{
		{"a copy's tip", copyable, `<span class="verso-tip group-hover/copy:opacity-100 group-focus-visible/copy:opacity-100 group-data-[copy-failed=true]/copy:opacity-100">`},
		{"a chip's explanation", explained, `class="verso-tip" data-verso-tip-kind="info"`},
	} {
		if got := render(t, r, c.w); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s in:\n%s", c.name, c.want, got)
		}
	}
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// Table presents rows of cell values beneath column headers. Cells are plain
// text for now; nested-widget cells will arrive once a second widget type
// exists to nest.
type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (*Table) isWidget() {}

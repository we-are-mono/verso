// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Grid arranges its children across columns — the side-by-side counterpart to stack
// (one column). It is a generic layout primitive that names no content: it composes
// a row of stat tiles, a set of service cards, anything.
//
// Columns is the target for wide screens; the plugin declares only that intent
// ("lay these out in N columns") and the shell owns the responsive behaviour,
// collapsing to fewer columns as the viewport narrows (ADR-005). The shell owns the
// gap too — a plugin never expresses spacing.
type Grid struct {
	Style    string
	Columns  int
	Children []Widget
}

func (*Grid) isWidget() {}

func (g *Grid) UnmarshalJSON(data []byte) error {
	var raw struct {
		Style    string            `json:"style"`
		Columns  int               `json:"columns"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Style = raw.Style
	g.Columns = raw.Columns
	g.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("grid child %d: %w", i, err)
		}
		g.Children = append(g.Children, w)
	}
	return nil
}

// gridView is the template model: the responsive column classes the shell chose for
// the declared count, plus the pre-rendered children.
type gridView struct {
	Cols     string
	Gap      string
	Children []template.HTML
}

func (g *Grid) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	gap := "gap-8"
	if g.Style == "form" {
		gap = "gap-6"
	}
	return r.execute(out, "grid.html.tmpl", gridView{Cols: gridCols(g.Columns, g.Style), Gap: gap, Children: children})
}

// gridCols maps a declared column count to responsive Tailwind classes — the shell's
// job of turning semantic intent into pixels (ADR-005). Small tile/card grids go
// two-up on phones (never a lonely full-width column) and widen on larger screens.
// The strings are static literals so Tailwind's compiler retains the classes.
func gridCols(n int, style string) string {
	if style == "form" {
		switch {
		case n <= 1:
			return "grid-cols-1"
		case n == 2:
			return "grid-cols-1 md:grid-cols-2"
		default:
			return "grid-cols-1 md:grid-cols-3"
		}
	}
	switch {
	case n <= 1:
		return "grid-cols-1"
	case n == 2:
		return "grid-cols-2"
	case n == 3:
		return "grid-cols-2 md:grid-cols-3"
	default: // 4 or more
		return "grid-cols-2 md:grid-cols-4"
	}
}

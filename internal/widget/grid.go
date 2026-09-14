// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
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

func (g *Grid) children() []Widget { return g.Children }

func (g *Grid) prune(keep func(Widget) bool) { g.Children = pruneList(g.Children, keep) }

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
	children, err := decodeChildren(raw.Children, "grid child")
	if err != nil {
		return err
	}
	g.Children = children
	return nil
}

// gridView is the template model: the style and the declared count, which the
// template turns into column classes, plus the pre-rendered children. The classes
// are the template's to state, not this file's — the stylesheet is built from the
// templates alone, so a class named in Go is a class the page never gets.
type gridView struct {
	Style    string
	Columns  int
	Children []template.HTML
}

func (g *Grid) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "grid.html.tmpl", gridView{Style: g.Style, Columns: g.Columns, Children: children})
}

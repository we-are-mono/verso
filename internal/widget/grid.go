// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
	"strings"
)

// Grid arranges its children across columns — the side-by-side counterpart to stack
// (one column). It is a generic layout primitive that names no content: it composes
// a row of stat tiles, a set of service cards, anything.
//
// Columns is the target for wide screens; the plugin declares only that intent
// ("lay these out in N columns") and the shell owns the responsive behaviour,
// collapsing to fewer columns as the viewport narrows (ADR-005). The shell owns the
// gap too — a plugin never expresses spacing.
// Style "form" groups related fields and responds to its container's width,
// so the same group can appear on a page or inside a drawer.
//
// Label names a "form" group whose values the shell fuses into one control:
// the words that cover every part ("Connection rate" over a rate and a burst),
// worn on the row's one label line beside the one chip naming every option.
// Help is what the group is, raised onto that label; without it the label
// raises each part's own explanation under the part's name.
type Grid struct {
	Style    string
	Columns  int
	Children []Widget
	Label    string
	Help     string
}

func (*Grid) isWidget() {}

func (g *Grid) children() []Widget { return g.Children }

func (g *Grid) prune(keep func(Widget) bool) { g.Children = pruneList(g.Children, keep) }

func (g *Grid) UnmarshalJSON(data []byte) error {
	var raw struct {
		Style    string            `json:"style"`
		Columns  int               `json:"columns"`
		Children []json.RawMessage `json:"children"`
		Label    string            `json:"label"`
		Help     string            `json:"help"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Style = raw.Style
	g.Columns = raw.Columns
	g.Label = raw.Label
	g.Help = raw.Help
	children, err := decodeChildren(raw.Children, "grid child")
	if err != nil {
		return err
	}
	g.Children = children
	return nil
}

// fusable reports whether a group of related fields is values typed into
// boxes, which the shell joins into one control (verso-box) — a secret and
// its repeat among them. A choice, a list, a range, a secret with its own
// reveal or a removable row is not one bare box, so a group holding one
// keeps its columns.
func (g *Grid) fusable() ([]*Field, bool) {
	if g.Style != "form" || len(g.Children) < 2 {
		return nil, false
	}
	fields := make([]*Field, 0, len(g.Children))
	for _, child := range g.Children {
		f, ok := child.(*Field)
		if !ok || (f.Kind != "" && f.Kind != "text" && f.Kind != "password") || f.Style != "" || f.Pair != nil || f.Remove != "" {
			return nil, false
		}
		fields = append(fields, f)
	}
	return fields, true
}

// renderFused draws related values as one row holding one box. The row is
// one setting: one label covering every part, one chip naming every option,
// one mark however many parts wait. Each part is still its own input, named
// by its own label for a screen reader, and a refused part's band says which
// part it is about.
func (r *Renderer) renderFused(out io.Writer, g *Grid, fields []*Field) error {
	id := fields[0].Name + "-group"
	label := fieldLabel{For: id, Group: true, Label: g.Label}
	var names, keys []string
	var box boxView
	var frame fieldFrame
	for _, f := range fields {
		box.Parts = append(box.Parts, boxPart{Field: f, Track: true, Named: true})
		names = append(names, f.Label)
		if f.Key != "" {
			keys = append(keys, f.Key)
		}
		if f.Staged {
			label.Staged = true
		}
		if g.Help == "" && f.Explained() {
			label.Tip.Parts = append(label.Tip.Parts, TipPart{Label: f.Label, Tip: f.explanation()})
		}
		if f.Error != "" {
			frame.Errors = append(frame.Errors, fieldError{ID: f.Name + "-error", Text: f.Label + ": " + f.Error})
		}
	}
	if label.Label == "" {
		label.Label = strings.Join(names, " · ")
	}
	label.Key = strings.Join(keys, " · ")
	label.Tip.ID, label.Tip.Tip, label.Tip.Footer = id+"-tip", g.Help, label.Key
	label.Explained = g.Help != "" || len(label.Tip.Parts) > 0
	box.Group = id + "-label"
	if label.Explained {
		box.Described = label.Tip.ID
	}
	frame.Label = label
	return r.renderFrame(out, "verso-box", box, frame)
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
	if fields, ok := g.fusable(); ok {
		return r.renderFused(out, g, fields)
	}
	children, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "grid.html.tmpl", gridView{Style: g.Style, Columns: g.Columns, Children: children})
}

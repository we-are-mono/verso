// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
	"slices"
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
//
// Join is the word between a "form" group's parts ("to"), for parts that
// read as one sentence — a path from one zone to another, a window from one
// time to the next. The word always splits them: each part stands as its own
// control, dropdowns and typed values alike, with the word between.
type Grid struct {
	Style    string
	Columns  int
	Children []Widget
	Label    string
	Help     string
	Join     string
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
		Join     string            `json:"join"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Style = raw.Style
	g.Columns = raw.Columns
	g.Label = raw.Label
	g.Help = raw.Help
	g.Join = raw.Join
	children, err := decodeChildren(raw.Children, "grid child")
	if err != nil {
		return err
	}
	g.Children = children
	return nil
}

// fusable reports whether a group of related fields is values typed into
// boxes, which the shell joins into one control (verso-box) — a secret and
// its repeat among them. A group joined by a word is never fused: the word
// splits it. A choice, a list, a secret with its own reveal or a removable
// row is not one bare box, so a group holding one keeps its columns.
func (g *Grid) fusable() ([]*Field, bool) {
	if g.Join != "" {
		return nil, false
	}
	return g.bareFields(func(f *Field) bool { return f.Kind == "" || f.Kind == "text" || f.Kind == "password" })
}

// joined reports whether a group of related fields reads as one sentence
// with a word between its parts — a path from one zone to another, a window
// from one time to the next. The word always splits them: each part stands
// as its own control, dropdowns and typed values alike, and a choice short
// enough for radios still stays a dropdown here, because a sentence is read
// across, not down.
func (g *Grid) joined() ([]*Field, bool) {
	if g.Join == "" {
		return nil, false
	}
	return g.bareFields(func(f *Field) bool {
		return f.Kind == "" || f.Kind == "text" || f.Kind == "select" || f.Kind == "time"
	})
}

// bareFields reports whether a "form" group of two or more is fields only,
// each of a kind the caller accepts, with no style or remove of its own.
func (g *Grid) bareFields(kind func(*Field) bool) ([]*Field, bool) {
	if g.Style != "form" || len(g.Children) < 2 {
		return nil, false
	}
	fields := make([]*Field, 0, len(g.Children))
	for _, child := range g.Children {
		f, ok := child.(*Field)
		if !ok || !kind(f) || f.Style != "" || f.Remove != "" {
			return nil, false
		}
		fields = append(fields, f)
	}
	return fields, true
}

// groupParts is a group's parts as one row's: every part tracked and named on
// its own, the group's word standing before every part after the first.
func groupParts(g *Grid, fields []*Field) boxView {
	var parts boxView
	for i, f := range fields {
		part := boxPart{Field: f, Track: true, Named: true}
		if i > 0 && g.Join != "" {
			part.Join, part.JoinID = g.Join, f.Name+"-join"
		}
		parts.Parts = append(parts.Parts, part)
	}
	return parts
}

// renderFused draws related values as one row holding one box.
func (r *Renderer) renderFused(out io.Writer, g *Grid, fields []*Field) error {
	box, frame := groupRow(g, fields, groupParts(g, fields))
	return r.renderFrame(out, "verso-box", box, frame)
}

// renderJoined draws a sentence of related fields as one row: each part its
// own control, side by side, the group's word between them.
func (r *Renderer) renderJoined(out io.Writer, g *Grid, fields []*Field) error {
	parts, frame := groupRow(g, fields, groupParts(g, fields))
	return r.renderFrame(out, "verso-joined", parts, frame)
}

// groupRow is the row a group of related fields stands in. The row is one
// setting: one label covering every part, one chip naming every option, one
// mark however many parts wait. Each part is still its own control, named by
// its own label for a screen reader, and a refused part's band says which
// part it is about.
func groupRow(g *Grid, fields []*Field, parts boxView) (boxView, fieldFrame) {
	id := fields[0].Name + "-group"
	label := fieldLabel{For: id, Group: true, Label: g.Label}
	var names, keys []string
	var frame fieldFrame
	for _, f := range fields {
		names = append(names, f.Label)
		// Parts that write one option between them (a rate's count and its
		// unit) name it once.
		if f.Key != "" && !slices.Contains(keys, f.Key) {
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
	parts.Group = id + "-label"
	if label.Explained {
		parts.Described = label.Tip.ID
	}
	frame.Label = label
	return parts, frame
}

// switchGroup reports whether a labelled group of related fields is switches
// only — one setting asked of several things, which files load — which the
// shell draws as one row: the label, then a checkbox row per switch.
func (g *Grid) switchGroup() ([]*Switch, bool) {
	if g.Style != "form" || g.Label == "" || len(g.Children) == 0 {
		return nil, false
	}
	switches := make([]*Switch, 0, len(g.Children))
	for _, child := range g.Children {
		s, ok := child.(*Switch)
		if !ok || s.Style == "inline" {
			return nil, false
		}
		switches = append(switches, s)
	}
	return switches, true
}

// switchGroupView is a switch group's checkbox rows and the id of the label
// that names them.
type switchGroupView struct {
	Group string
	Rows  []template.HTML
}

// renderSwitchGroup draws a labelled group of switches as one setting's row:
// the group's label on top with the options its switches write named once,
// and under it one checkbox row per switch, each with its own words, its own
// change and its own mark.
func (r *Renderer) renderSwitchGroup(out io.Writer, g *Grid, switches []*Switch) error {
	id := switches[0].Name + "-group"
	view := switchGroupView{Group: id + "-label"}
	var keys []string
	seen := map[string]bool{}
	for _, s := range switches {
		if s.Key != "" && !seen[s.Key] {
			seen[s.Key] = true
			keys = append(keys, s.Key)
		}
		control, frame := s.grouped()
		var row strings.Builder
		if err := r.renderFrame(&row, "switch.row", control, frame); err != nil {
			return err
		}
		view.Rows = append(view.Rows, template.HTML(row.String()))
	}
	key := strings.Join(keys, " · ")
	label := fieldLabel{
		For: id, Group: true, Label: g.Label, Key: key, Explained: g.Help != "",
		Tip: TipView{ID: id + "-tip", Tip: g.Help, Footer: key},
	}
	return r.renderFrame(out, "verso-switch-group", view, fieldFrame{Label: label})
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
	if fields, ok := g.joined(); ok {
		return r.renderJoined(out, g, fields)
	}
	if switches, ok := g.switchGroup(); ok {
		return r.renderSwitchGroup(out, g, switches)
	}
	children, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "grid.html.tmpl", gridView{Style: g.Style, Columns: g.Columns, Children: children})
}

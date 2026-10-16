// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
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
	Style    string  `json:"style"`
	Columns  int     `json:"columns"`
	Children Widgets `json:"children"`
	Label    string  `json:"label"`
	Help     string  `json:"help"`
	Join     string  `json:"join"`
}

func (*Grid) isWidget() {}

func (g *Grid) children() []Widget { return g.Children }

func (g *Grid) prune(keep func(Widget) bool) { g.Children = pruneList(g.Children, keep) }

// fusable reports whether a group of related fields is values typed into
// boxes, which the shell joins into one control (verso-box) — a secret and
// its repeat among them, or a value and the unit it is counted in. A group
// joined by a word is never fused: the word splits it. A list, a secret with
// its own reveal or a removable row is not one bare box, so a group holding
// one stands as a row per field.
func (g *Grid) fusable() ([]*Field, bool) {
	if g.Join != "" {
		return nil, false
	}
	if fields, ok := g.bareFields(func(f *Field) bool { return f.Kind == "" || f.Kind == "text" || f.Kind == "password" }); ok {
		return fields, true
	}
	return g.valueAndUnit()
}

// valueAndUnit reports whether the group is a typed value and a dropdown of
// the units it is counted in ("10", "minutes"): one value said in two
// options, which fuses into one box under the value's own name.
func (g *Grid) valueAndUnit() ([]*Field, bool) {
	fields, ok := g.bareFields(func(f *Field) bool { return f.Kind == "" || f.Kind == "text" || f.Kind == "select" })
	if !ok || !isValueAndUnit(fields) {
		return nil, false
	}
	return fields, true
}

// isValueAndUnit reports whether fields are a typed value, then its unit.
func isValueAndUnit(fields []*Field) bool {
	return len(fields) == 2 && fields[0].Kind != "select" && fields[1].Kind == "select"
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

// groupParts is a group's parts as one row's: every part named on its own and
// tracked by the stage unless it fetches a view instead, the group's word
// standing before every part after the first.
func groupParts(g *Grid, fields []*Field) boxView {
	var parts boxView
	for i, f := range fields {
		part := boxPart{Field: f, Track: f.Fetch == nil, Named: true}
		if i > 0 && g.Join != "" {
			part.Join, part.JoinID = g.Join, f.Name+"-join"
		}
		parts.Parts = append(parts.Parts, part)
	}
	return parts
}

// renderFused draws related values as one row holding one box. In a labelled
// set each typed part names itself inside the box, where a unit stands, since
// the row's name is the set's; a value fused with its unit is named by the
// value's own name, and its parts need none.
func (r *Renderer) renderFused(out io.Writer, g *Grid, fields []*Field) error {
	parts := groupParts(g, fields)
	if g.Label != "" && !isValueAndUnit(fields) {
		for i := range parts.Parts {
			parts.Parts[i].Shown = parts.Parts[i].Kind != "select" && parts.Parts[i].Unit == ""
		}
	}
	box, frame := groupRow(g, fields, parts)
	frame.Wide = partsWidth(box.Parts, 1) > answerColumn
	return r.renderFrame(out, "verso-box", box, frame)
}

// renderJoined draws a sentence of related fields as one row: each part its
// own control, side by side, the group's word between them.
func (r *Renderer) renderJoined(out io.Writer, g *Grid, fields []*Field) error {
	parts, frame := groupRow(g, fields, groupParts(g, fields))
	frame.Wide = partsWidth(parts.Parts, joinerWidth(g.Join)) > answerColumn
	return r.renderFrame(out, "verso-joined", parts, frame)
}

// sansRune is roughly how wide a character of the 14px sans stands, and
// monoRune one of the 16px mono, in pixels: enough to tell a group that fits
// the answer column from one that does not.
const (
	sansRune = 7
	monoRune = 8
)

// partsWidth is roughly how wide a group's parts stand side by side, between
// stands the gap (a fused box's inner hairline, a joined row's word).
func partsWidth(parts []boxPart, between int) int {
	width := 0
	for i, p := range parts {
		if i > 0 {
			width += between
		}
		width += partWidth(p.Field)
		if p.Shown {
			width += utf8.RuneCountInString(p.Label)*sansRune + 12
		}
	}
	return width
}

// partWidth is roughly how wide one part stands: a dropdown its longest
// option and its chevron, a clock its own box, a typed value its measure, and
// a unit's word after its value.
func partWidth(f *Field) int {
	if f.Kind == "select" {
		longest := 0
		for _, o := range f.Options {
			longest = max(longest, utf8.RuneCountInString(o.Label))
		}
		return 12 + longest*sansRune + 36
	}
	width := map[string]int{"number": 96, "short": 128, "secret": 256}[boxPart{Field: f}.Measure()]
	switch {
	case f.Kind == "time":
		width = 128
	case width == 0:
		width = 176
	}
	if f.Unit != "" {
		width += utf8.RuneCountInString(f.Unit)*sansRune + 12
	}
	return width
}

// joinerWidth is how wide a joined row's word stands with its 12px either
// side.
func joinerWidth(word string) int { return 24 + utf8.RuneCountInString(word)*sansRune }

// groupRow is the row a group of related fields stands in. The row is one
// setting: one label covering every part, one chip naming every option, one
// mark however many parts wait, and one description: the group's help, then
// each part's own led by its name. Each part is still its own control, named
// by its own label for a screen reader, and a refused part's band says which
// part it is about.
func groupRow(g *Grid, fields []*Field, parts boxView) (boxView, fieldFrame) {
	id := fields[0].Name + "-group"
	label := fieldLabel{For: id, Group: true, Label: g.Label}
	var names, keys, said, alone []string
	// A group of choices of what to look at is no setting; one setting among
	// its parts makes it one.
	frame := fieldFrame{Plain: true}
	for _, f := range fields {
		if f.Fetch == nil {
			frame.Plain = false
		}
		names = append(names, f.Label)
		// Parts that write one option between them (a rate's count and its
		// unit) name it once.
		if f.Key != "" && !slices.Contains(keys, f.Key) {
			keys = append(keys, f.Key)
		}
		if f.Staged {
			label.Staged = true
			if label.StagedAt == "" {
				label.StagedAt = f.StagedAt
			}
		}
		if d := f.description(); d != "" {
			said, alone = append(said, f.Label+": "+d), append(alone, d)
		}
		if f.Error != "" {
			frame.Errors = append(frame.Errors, fieldError{ID: f.Name + "-error", Text: f.Label + ": " + f.Error})
		}
	}
	switch {
	case label.Label != "":
	case isValueAndUnit(fields):
		// A value and its unit are named by the value.
		label.Label = fields[0].Label
	default:
		label.Label = strings.Join(names, " · ")
	}
	label.Key = strings.Join(keys, " · ")
	frame.Label = label
	// A part that is the only one saying anything speaks for the row, so its
	// name in front would only repeat the label over it.
	if g.Help == "" && len(alone) == 1 {
		said = alone
	}
	frame.Desc = sentences(append([]string{g.Help}, said...)...)
	parts.Group = id + "-label"
	if frame.Desc != "" {
		parts.Described = frame.DescID()
	}
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

// rows reports whether a "form" group is settings only — fields, switches,
// lists, gates — each of which stands as a row of its own, and the name of
// the first, which names the group.
func (g *Grid) rows() (string, bool) {
	if g.Style != "form" || len(g.Children) == 0 {
		return "", false
	}
	first := ""
	for i, child := range g.Children {
		var name string
		switch w := child.(type) {
		case *Field:
			name = w.Name
		case *Switch:
			name = w.Name
		case *List:
			name = w.Name
		case *Conditional:
			name = w.Name
		default:
			return "", false
		}
		if i == 0 {
			first = name
		}
	}
	return first, true
}

// fieldSetView is a field set: the id of the label that names it, its head
// row (the label, the chip and the description) and its rows under it.
type fieldSetView struct {
	Group string
	Head  template.HTML
	Rows  []template.HTML
}

// renderSwitchGroup draws a labelled group of switches as one setting: a head
// row, the group's label with the options its switches write named once and
// the group's description, and under it one row per switch, each with its own
// name, detail, change and mark, its box in the answer column.
func (r *Renderer) renderSwitchGroup(out io.Writer, g *Grid, switches []*Switch) error {
	var keys []string
	var rows []template.HTML
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
		rows = append(rows, template.HTML(row.String()))
	}
	return r.renderFieldSet(out, g, switches[0].Name+"-group", strings.Join(keys, " · "), rows)
}

// renderRows draws a group of settings that cannot fuse as a row per
// setting, one under another, never side by side: a set the group's label or
// help heads is a field set, its rows under the head; a set with neither
// joins the form's own run of rows.
func (r *Renderer) renderRows(out io.Writer, g *Grid, first, csrf string) error {
	rows, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	if g.Label != "" || g.Help != "" {
		return r.renderFieldSet(out, g, first+"-group", "", rows)
	}
	for _, row := range rows {
		if _, err := io.WriteString(out, string(row)); err != nil {
			return err
		}
	}
	return nil
}

// renderFieldSet draws rows under one head: the set's label (which names the
// set for a screen reader too), the chip naming the option its rows write
// once, and the set's description.
func (r *Renderer) renderFieldSet(out io.Writer, g *Grid, id, key string, rows []template.HTML) error {
	view := fieldSetView{Rows: rows}
	if g.Label != "" {
		view.Group = id + "-label"
	}
	label := fieldLabel{For: id, Group: true, Label: g.Label, Key: key}
	var head strings.Builder
	if err := r.renderFrame(&head, "verso-nothing", nil, fieldFrame{Label: label, Desc: g.Help}); err != nil {
		return err
	}
	view.Head = template.HTML(head.String())
	return r.execute(out, "verso-field-set", view)
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
	if first, ok := g.rows(); ok {
		return r.renderRows(out, g, first, csrf)
	}
	children, err := r.renderChildren(g.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "grid.html.tmpl", gridView{Style: g.Style, Columns: g.Columns, Children: children})
}

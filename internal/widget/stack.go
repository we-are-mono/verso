// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Stack lays its children out without drawing a surface of its own. By default it
// spaces them vertically — the container for a page of separate cards, where card
// draws the box. Compact tightens that rhythm and Loose opens it; Divided draws a
// hairline between entries; Inline forms a wrapping action/content row.
type Stack struct {
	Divided bool
	Compact bool
	Loose   bool
	Inline  bool
	// Flush adds no rhythm of its own, for a run of children that already carry
	// their own — form rows, which each hold 12px above and below themselves.
	// Any spacing here would be added to theirs and a form would read as a list
	// of separate things rather than as one set of settings.
	Flush    bool
	Width    string
	Children []Widget
}

func (*Stack) isWidget() {}

func (s *Stack) children() []Widget { return s.Children }

func (s *Stack) prune(keep func(Widget) bool) { s.Children = pruneList(s.Children, keep) }

func (s *Stack) UnmarshalJSON(data []byte) error {
	var raw struct {
		Divided  bool              `json:"divided"`
		Compact  bool              `json:"compact"`
		Loose    bool              `json:"loose"`
		Flush    bool              `json:"flush"`
		Inline   bool              `json:"inline"`
		Width    string            `json:"width"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Divided = raw.Divided
	s.Compact = raw.Compact
	s.Loose = raw.Loose
	s.Flush = raw.Flush
	s.Inline = raw.Inline
	s.Width = raw.Width
	children, err := decodeChildren(raw.Children, "stack child")
	if err != nil {
		return err
	}
	s.Children = children
	return nil
}

type stackView struct {
	Divided  bool
	Compact  bool
	Loose    bool
	Flush    bool
	Inline   bool
	Width    string
	Children []template.HTML
}

// renderInto renders each child through the renderer and applies the requested
// vertical, divided, or inline rhythm.
func (s *Stack) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "stack.html.tmpl", stackView{Divided: s.Divided, Compact: s.Compact, Loose: s.Loose, Flush: s.Flush, Inline: s.Inline, Width: s.Width, Children: children})
}

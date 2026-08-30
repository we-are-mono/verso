// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Stack lays its children out without drawing a surface of its own. By default it
// spaces them vertically — the container for a page of separate cards, where card
// draws the box. Compact tightens that rhythm; Divided draws a hairline between
// entries; Inline forms a wrapping action/content row.
type Stack struct {
	Divided  bool
	Compact  bool
	Inline   bool
	Children []Widget
}

func (*Stack) isWidget() {}

func (s *Stack) UnmarshalJSON(data []byte) error {
	var raw struct {
		Divided  bool              `json:"divided"`
		Compact  bool              `json:"compact"`
		Inline   bool              `json:"inline"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Divided = raw.Divided
	s.Compact = raw.Compact
	s.Inline = raw.Inline
	s.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("stack child %d: %w", i, err)
		}
		s.Children = append(s.Children, w)
	}
	return nil
}

type stackView struct {
	Divided  bool
	Compact  bool
	Inline   bool
	Children []template.HTML
}

// renderInto renders each child through the renderer and applies the requested
// vertical, divided, or inline rhythm.
func (s *Stack) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "stack.html.tmpl", stackView{Divided: s.Divided, Compact: s.Compact, Inline: s.Inline, Children: children})
}

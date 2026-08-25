// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Stack lays its children out vertically and draws no surface of its own. By default
// it spaces them evenly — the container for a page of separate cards, where card draws
// the box. Divided instead draws a hairline between each child, for a list of entries
// (a device roster, a settings list) rather than a page of cards.
type Stack struct {
	Divided  bool
	Children []Widget
}

func (*Stack) isWidget() {}

func (s *Stack) UnmarshalJSON(data []byte) error {
	var raw struct {
		Divided  bool              `json:"divided"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Divided = raw.Divided
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
	Children []template.HTML
}

// renderInto renders each child through the renderer and lays them out vertically —
// spaced, or hairline-divided when Divided.
func (s *Stack) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "stack.html.tmpl", stackView{Divided: s.Divided, Children: children})
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"io"
)

// Stack lays its children out vertically with consistent spacing and draws no
// surface of its own — the container for a page of separate cards, where card
// draws the box. It is the page-level counterpart to card's grouped box.
type Stack struct {
	Children []Widget
}

func (*Stack) isWidget() {}

func (s *Stack) UnmarshalJSON(data []byte) error {
	var raw struct {
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
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

// renderInto renders each child through the renderer and lays them out vertically.
func (s *Stack) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "stack.html.tmpl", children)
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Section groups related widgets under an optional heading, with generous space
// above it — a new context on the page, set apart so the eye registers the shift.
// Use it to break a long page into scannable parts (a device list, an advanced
// block) rather than one undifferentiated run of widgets.
type Section struct {
	Title    string   `json:"title"`
	Children []Widget `json:"children"`
}

func (*Section) isWidget() {}

// UnmarshalJSON decodes the contents recursively through Decode, so an unknown
// child type fails loudly rather than vanishing.
func (s *Section) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title    string            `json:"title"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Title = raw.Title
	s.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("section child %d: %w", i, err)
		}
		s.Children = append(s.Children, w)
	}
	return nil
}

type sectionView struct {
	Title    string
	Children []template.HTML
}

func (s *Section) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "section.html.tmpl", sectionView{Title: s.Title, Children: children})
}

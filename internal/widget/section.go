// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Section groups related widgets under an optional heading, with generous space
// above it — a new context on the page, set apart so the eye registers the shift.
// Use it to break a long page into scannable parts (a device list, an advanced
// block) rather than one undifferentiated run of widgets.
//
// Sub is an optional one-or-two-sentence description (Markdown) that belongs to
// the heading: it renders tight beneath the title, and the gap to the section's
// first child stays where it was — the head reads as one unit, the content as
// another. Meta is optional, compact status text aligned opposite the title;
// MetaLabel can identify that value without folding the label into it, and
// MetaIcon names a shell-owned Lucide glyph rendered immediately before both.
// Hairline is opt-in: when true, a section following another section gets a
// divider and additional breathing room above it.
type Section struct {
	Title     string   `json:"title"`
	Sub       string   `json:"sub,omitempty"`
	Meta      string   `json:"meta,omitempty"`
	MetaLabel string   `json:"meta_label,omitempty"`
	MetaIcon  string   `json:"meta_icon,omitempty"`
	Hairline  bool     `json:"hairline,omitempty"`
	Children  []Widget `json:"children"`
}

func (*Section) isWidget() {}

// UnmarshalJSON decodes the contents recursively through Decode, so an unknown
// child type fails loudly rather than vanishing.
func (s *Section) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title     string            `json:"title"`
		Sub       string            `json:"sub"`
		Meta      string            `json:"meta"`
		MetaLabel string            `json:"meta_label"`
		MetaIcon  string            `json:"meta_icon"`
		Hairline  bool              `json:"hairline"`
		Children  []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Title = raw.Title
	s.Sub = raw.Sub
	s.Meta = raw.Meta
	s.MetaLabel = raw.MetaLabel
	s.MetaIcon = raw.MetaIcon
	s.Hairline = raw.Hairline
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
	Title     string
	Sub       template.HTML
	Meta      string
	MetaLabel string
	MetaIcon  string
	Hairline  bool
	Children  []template.HTML
}

func (s *Section) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	v := sectionView{
		Title:     s.Title,
		Meta:      s.Meta,
		MetaLabel: s.MetaLabel,
		MetaIcon:  s.MetaIcon,
		Hairline:  s.Hairline,
		Children:  children,
	}
	if s.Sub != "" {
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(s.Sub), &buf); err != nil {
			return fmt.Errorf("widget: render section sub: %w", err)
		}
		v.Sub = template.HTML(buf.String())
	}
	return r.execute(out, "section.html.tmpl", v)
}

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
// MetaPosition may be "inline" when the meta belongs directly to the title;
// the shell separates the two with a centred dot. The default keeps meta on the
// opposite edge of the title row.
// Control is an optional compact interactive widget placed directly beside the
// title, for object-level state such as an enabled switch. Flush removes the
// section's own top inset when its parent already supplies the outer padding.
// Hairline is opt-in: when true, a section following another section gets a
// divider and additional breathing room above it.
type Section struct {
	Title        string   `json:"title"`
	Sub          string   `json:"sub,omitempty"`
	Meta         string   `json:"meta,omitempty"`
	MetaLabel    string   `json:"meta_label,omitempty"`
	MetaIcon     string   `json:"meta_icon,omitempty"`
	MetaPosition string   `json:"meta_position,omitempty"`
	Hairline     bool     `json:"hairline,omitempty"`
	Flush        bool     `json:"flush,omitempty"`
	Control      Widget   `json:"-"`
	Children     []Widget `json:"children"`
}

func (*Section) isWidget() {}

func (s *Section) children() []Widget {
	if s.Control == nil {
		return s.Children
	}
	return append([]Widget{s.Control}, s.Children...)
}

// UnmarshalJSON decodes the contents recursively through Decode, so an unknown
// child type fails loudly rather than vanishing.
func (s *Section) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title        string            `json:"title"`
		Sub          string            `json:"sub"`
		Meta         string            `json:"meta"`
		MetaLabel    string            `json:"meta_label"`
		MetaIcon     string            `json:"meta_icon"`
		MetaPosition string            `json:"meta_position"`
		Hairline     bool              `json:"hairline"`
		Flush        bool              `json:"flush"`
		Control      json.RawMessage   `json:"control"`
		Children     []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Title = raw.Title
	s.Sub = raw.Sub
	s.Meta = raw.Meta
	s.MetaLabel = raw.MetaLabel
	s.MetaIcon = raw.MetaIcon
	s.MetaPosition = raw.MetaPosition
	s.Hairline = raw.Hairline
	s.Flush = raw.Flush
	s.Control = nil
	if len(raw.Control) != 0 && string(raw.Control) != "null" {
		control, err := Decode(raw.Control)
		if err != nil {
			return fmt.Errorf("section control: %w", err)
		}
		s.Control = control
	}
	children, err := decodeChildren(raw.Children, "section child")
	if err != nil {
		return err
	}
	s.Children = children
	return nil
}

type sectionView struct {
	Title      string
	Sub        template.HTML
	Meta       string
	MetaLabel  string
	MetaIcon   string
	MetaInline bool
	Hairline   bool
	Flush      bool
	Control    template.HTML
	Children   []template.HTML
}

func (s *Section) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	v := sectionView{
		Title:      s.Title,
		Meta:       s.Meta,
		MetaLabel:  s.MetaLabel,
		MetaIcon:   s.MetaIcon,
		MetaInline: s.MetaPosition == "inline",
		Hairline:   s.Hairline,
		Flush:      s.Flush,
		Children:   children,
	}
	if s.Control != nil {
		var buf bytes.Buffer
		if err := r.render(&buf, s.Control, csrf); err != nil {
			return fmt.Errorf("widget: render section control: %w", err)
		}
		v.Control = template.HTML(buf.String())
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

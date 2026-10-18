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
// MetaLabel can identify that value without folding the label into it.
// MetaPosition may be "inline" when the meta belongs directly to the title;
// the shell separates the two with a centred dot. The default keeps meta on the
// opposite edge of the title row.
// Control is an optional compact interactive widget placed directly beside the
// title, for object-level state such as an enabled switch. Flush removes the
// section's own top inset when its parent already supplies the outer padding.
// Hairline is opt-in: when true, a section following another section gets a
// divider and additional breathing room above it.
type Section struct {
	Title string `json:"title"`
	// Icon is the glyph of what the section is, by Lucide name, leading its
	// title in a page card's header or on a part's ledger line (an expanded
	// row's network, its DHCP server, the bridge under them), so subjects of
	// one kind are told apart from another at a glance. It is identity, never
	// state.
	Icon string `json:"icon,omitempty"`
	// Anchor is the name this section is addressed by — what a link elsewhere on
	// the page points at to bring it into view. A page long enough to need a list
	// of its own parts needs them addressable; without this the list is a row of
	// links to nowhere.
	Anchor string `json:"anchor,omitempty"`
	// Kicker makes Title a small label over a group rather than a heading over a
	// subject — a rail's "On this page", where the words name the list beneath
	// them and must not compete with the headings the list points at.
	Kicker bool `json:"kicker,omitempty"`
	// Chip names which one of its kind a card is (a fiber module's cage), in
	// the shell's chip beside the title, verbatim. The shell composes it; it
	// is no part of the plugin contract.
	Chip string `json:"-"`
	Sub  string `json:"sub,omitempty"`
	Meta string `json:"meta,omitempty"`
	// MetaVerbatim declares Meta a machine value or an already-composed string
	// — data the localization walk leaves exactly as authored. Prose meta
	// stays undeclared and translates.
	MetaVerbatim bool   `json:"-"`
	MetaLabel    string `json:"meta_label,omitempty"`
	MetaPosition string `json:"meta_position,omitempty"`
	Hairline     bool   `json:"hairline,omitempty"`
	Flush        bool   `json:"flush,omitempty"`
	// Target is where the options this section's controls write live,
	// "config.section", when they all live in one place — said once here
	// rather than on each control (MarkStaged).
	Target   string  `json:"target,omitempty"`
	Control  Widget  `json:"-"`
	Children Widgets `json:"children"`
}

func (*Section) isWidget() {}

func (s *Section) children() []Widget {
	if s.Control == nil {
		return s.Children
	}
	return append([]Widget{s.Control}, s.Children...)
}

// UnmarshalJSON decodes the lone control through Decode, so an unknown control
// type fails loudly rather than vanishing; the children are Widgets.
func (s *Section) UnmarshalJSON(data []byte) error {
	type plain Section
	var raw struct {
		plain
		Control json.RawMessage `json:"control"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	control, err := decodeOptional(raw.Control)
	if err != nil {
		return fmt.Errorf("section control: %w", err)
	}
	*s = Section(raw.plain)
	s.Control = control
	return nil
}

// SectionID is the id a section with this anchor carries, and what a link to
// "#anchor" on the same page is sent to. A plugin names its anchors after the
// settings they hold, and so after the fields in them; the namespace keeps the
// section's id from ever being a field's too, which would leave that field's
// label pointing at the section.
func SectionID(anchor string) string {
	if anchor == "" {
		return ""
	}
	return "section-" + anchor
}

type sectionView struct {
	// Level is a nested part's heading level: one under the nearest titled
	// section above it, h2 where none stands over it (Renderer.headed).
	Level      int
	Title      string
	Icon       string
	Anchor     string
	Kicker     bool
	Chip       string
	Sub        template.HTML
	Meta       string
	MetaLabel  string
	MetaInline bool
	Hairline   bool
	Flush      bool
	// Nested marks a section inside another section: a part of that subject,
	// titled at h3 on a ledger line, not a second heading beside its parent's.
	Nested bool
	// OnCards says the surface it stands on stands its subjects on cards
	// (Renderer.cards).
	OnCards  bool
	Control  template.HTML
	Children []template.HTML
}

// Headless is a section with nothing over its content: no title, no meta, no
// act.
func (v sectionView) Headless() bool {
	return v.Title == "" && v.Meta == "" && v.Control == ""
}

// Kind is what the section is to the surface it stands on, which is all the
// markup says; its block turns it into air and a rule. On a surface of cards
// a subject with a heading, or a ruled one with none, is a card. A ruled
// section is one of the surface's subjects, a part is a ruled section inside
// another, a continued one is more of the rows above (its rule always drawn),
// a plain one is an unruled block, and a bare one — nested or flush — adds no
// air.
func (v sectionView) Kind() string {
	switch {
	case v.OnCards && !v.Nested && ((v.Title != "" && !v.Kicker) || (v.Headless() && v.Hairline && !v.Flush)):
		return "card"
	case v.Hairline && v.Flush:
		return "continued"
	case v.Hairline && v.Nested:
		return "part"
	case v.Hairline:
		return "ruled"
	case v.Flush || v.Nested:
		return "bare"
	}
	return "plain"
}

// postsWithForm is the heading's act as drawn. A section that saves itself
// holds its one form, and the act on its heading stands outside that form; a
// named act — one that posts — posts with it all the same, its value with the
// form's fields, so an act on a part of a card that saves itself reads what the
// card holds. The form is named for it here; the act is a copy, so the schema
// says what the plugin sent.
func (s *Section) postsWithForm(r *Renderer) Widget {
	act, ok := s.Control.(*Button)
	if !ok || act.Name == "" {
		return s.Control
	}
	var form *Form
	for _, child := range s.Children {
		if f, ok := child.(*Form); ok {
			if form != nil {
				return s.Control
			}
			form = f
		}
	}
	if form == nil {
		return s.Control
	}
	if form.ID == "" {
		form.ID = fmt.Sprintf("verso-form-%d", r.seq.form.Add(1))
	}
	posting := *act
	posting.Form = form.ID
	return &posting
}

func (s *Section) renderInto(r *Renderer, out io.Writer, csrf string) error {
	control := s.postsWithForm(r)
	v := sectionView{
		Nested:     r.depth > 0,
		OnCards:    r.cards,
		Level:      min(2+r.headed, 4),
		Title:      s.Title,
		Icon:       s.Icon,
		Anchor:     SectionID(s.Anchor),
		Kicker:     s.Kicker,
		Chip:       s.Chip,
		Meta:       s.Meta,
		MetaLabel:  s.MetaLabel,
		MetaInline: s.MetaPosition == "inline",
		Hairline:   s.Hairline,
		Flush:      s.Flush,
	}
	if control != nil {
		var buf bytes.Buffer
		if err := r.render(&buf, control, csrf); err != nil {
			return fmt.Errorf("widget: render section control: %w", err)
		}
		v.Control = template.HTML(buf.String())
	}
	// What a card holds stands straight in its body.
	pass := r.nested(s.Title != "")
	pass.inCard = v.Kind() == "card"
	children, err := pass.renderChildren(s.Children, csrf)
	if err != nil {
		return err
	}
	v.Children = children
	if s.Sub != "" {
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(s.Sub), &buf); err != nil {
			return fmt.Errorf("widget: render section sub: %w", err)
		}
		v.Sub = template.HTML(buf.String())
	}
	return r.execute(out, "section.html.tmpl", v)
}

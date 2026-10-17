// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// A drawer's body is a page's ground in small: its subjects stand as Paper
// cards on Quiet Sand, as a page's do (sections.css). A plugin draws a drawer
// as it draws a form — a run of fields, its titled sections, the
// configuration card — and the shell stands that on cards. Whatever stands
// loose between the cards goes into an untitled card, as a page form's
// untitled run of fields does, so no field stands loose on the sand. What is
// a card already (a titled or ruled section, the configuration card) stays as
// the plugin sent it, and what speaks for the whole drawer or acts on it (a
// notice, a removal's confirmation) stays on the sand between the cards, as a
// page's own notice and Save do. The plugin's tree is never rewritten: the
// containers on the way are copied.

// OnCards stands a tab's body, drawn into the entity panel's drawer, on
// cards.
func OnCards(w Widget) Widget {
	if w == nil {
		return nil
	}
	return onCards([]Widget{w})[0]
}

// onCards stands a drawer's top-level widgets on cards: each loose run in an
// untitled card, the cards and the sand's own widgets as they are, and the
// stacked forms and stacks that hold them stood on cards in turn.
func onCards(ws []Widget) []Widget {
	out := make([]Widget, 0, len(ws))
	var run []Widget
	drawn := false
	settle := func() {
		if drawn {
			out = append(out, &Section{Hairline: true, Children: run})
		} else {
			out = append(out, run...)
		}
		run, drawn = nil, false
	}
	for _, w := range ws {
		if held, ok := holdsCards(w); ok {
			settle()
			out = append(out, held)
			continue
		}
		if c, ok := asCard(w); ok {
			settle()
			out = append(out, liftFile(c)...)
			continue
		}
		if onSand(w) {
			settle()
			out = append(out, w)
			continue
		}
		run = append(run, w)
		if f, ok := w.(*Field); !ok || f.Kind != "hidden" {
			drawn = true
		}
	}
	settle()
	return out
}

// holdsCards is a container a drawer's cards stand in rather than a card of
// its own: a stacked form (its <form> holds them, and its Save follows the
// last) or a stack of blocks, copied with its contents stood on cards. A form
// of another style (a search, an inline row) is content, and so is a stack
// that is itself content: a row of acts, a divided list, a tight group, or a
// flush run of rows.
func holdsCards(w Widget) (Widget, bool) {
	switch v := w.(type) {
	case *Form:
		if v.Style != "" && v.Style != "page" && v.Style != "settings" {
			return nil, false
		}
		f := *v
		f.Fields = onCards(v.Fields)
		return &f, true
	case *Stack:
		if v.Inline || v.Divided || v.Compact || v.Flush {
			return nil, false
		}
		s := *v
		s.Children = onCards(v.Children)
		return &s, true
	}
	return nil, false
}

// asCard is a widget that already stands as a card: a section with a heading
// (its band is the card's header), an untitled ruled one, and a configuration
// card. The untitled block a drawer opens on, sent flush because the chrome's
// hairline used to be its rule, is the drawer's first card: copied as an
// untitled ruled section.
func asCard(w Widget) (Widget, bool) {
	switch v := w.(type) {
	case *Section:
		headless := v.Title == "" && v.Meta == "" && v.Control == nil
		switch {
		case !headless:
			return v, true
		case v.Hairline && !v.Flush:
			return v, true
		case v.Flush && !v.Hairline:
			s := *v
			s.Flush, s.Hairline = false, true
			return &s, true
		}
	case *Code:
		if v.drawsCard() {
			return v, true
		}
	}
	return nil, false
}

// liftFile stands the file a card's section ends on (a reading's
// configuration card, written as the last thing in its section) after that
// card as a card of its own, since no card holds a card: the section is
// copied without it, keeping the hidden carriers after it where they were.
func liftFile(w Widget) []Widget {
	s, ok := w.(*Section)
	if !ok {
		return []Widget{w}
	}
	for i := len(s.Children) - 1; i >= 0; i-- {
		if f, ok := s.Children[i].(*Field); ok && f.Kind == "hidden" {
			continue
		}
		file, ok := s.Children[i].(*Code)
		if !ok || !file.drawsCard() {
			break
		}
		held := *s
		held.Children = append(append([]Widget{}, s.Children[:i]...), s.Children[i+1:]...)
		return []Widget{&held, file}
	}
	return []Widget{w}
}

// onSand is what stands between a drawer's cards rather than in one: a notice
// about the whole drawer and an act on its object.
func onSand(w Widget) bool {
	switch w.(type) {
	case *Callout, *Confirm:
		return true
	}
	return false
}

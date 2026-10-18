// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Slot is a place a plugin leaves on its page for a block the shell owns
// (ADR-005): the plugin decides where the block stands, and the shell decides
// what it is and answers for it. The ids are the shell's, a closed set; a slot
// the shell has no block for renders nothing, so a plugin placing one an older
// shell does not know loses only that block.
//
// A slot with a Kind is one the plugin opens for other plugins to fill
// (ADR-026), named <plugin id>.<ID>: the shell fills it with what they
// contribute, each in its own plugin's words.
type Slot struct {
	ID string `json:"id"`
	// Kind is what an opened slot takes: "block", "act" or "notice". A facts
	// slot rides its list instead (Properties.Slot).
	Kind string `json:"kind,omitempty"`
	// fill is the block the shell put in the slot before rendering.
	fill Widget
}

func (*Slot) isWidget() {}

// Opened reports whether the slot is one its plugin opens for others, of a
// kind a slot widget holds; one of any other kind draws nothing.
func (s *Slot) Opened() bool {
	switch s.Kind {
	case "block", "act", "notice":
		return true
	}
	return false
}

// children is none: a slot is filled with a block the shell has already
// rendered in its own words, which no later pass over the plugin's tree may
// translate or mark again.
func (*Slot) children() []Widget { return nil }

// Fill puts the shell's block in the slot.
func (s *Slot) Fill(w Widget) { s.fill = w }

func (s *Slot) renderInto(r *Renderer, out io.Writer, csrf string) error {
	if s.fill == nil {
		return nil
	}
	return r.render(out, s.fill, csrf)
}

// Slots are the slots in w, in page order, for the shell to fill.
func Slots(w Widget) []*Slot {
	var slots []*Slot
	Walk(w, func(n Widget) {
		if s, ok := n.(*Slot); ok {
			slots = append(slots, s)
		}
	})
	return slots
}

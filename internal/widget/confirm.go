// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
)

// Confirm guards a consequential action with an inline confirmation: a trigger
// that, when pressed, swaps in place for a short "are you sure?" with a cancel and
// a confirm — no separate dialog, no leaving the spot. The whole exchange speaks
// one hue, chosen by Tone.
type Confirm struct {
	Trigger string `json:"trigger"` // the act's own name, e.g. "Remove device"
	// Title is an optional bold heading above the message — for a consequence
	// too long to read as one paragraph, the heading asks the question and the
	// message explains it. Absent, the message stands alone as it always has.
	Title           string `json:"title"`
	Message         string `json:"message"`          // the confirmation prompt
	Confirm         string `json:"confirm"`          // confirm-button label (default: the Trigger's own name)
	Cancel          string `json:"cancel"`           // cancel label (default "Cancel")
	RequirePassword bool   `json:"require_password"` // ask for the current administrator password
	// Tone is what the act costs. "caution" is disruptive but wanted — installing
	// firmware — and asks in marigold, so it steadies rather than scares. Anything
	// else is danger: the act destroys something, and asks in crimson.
	Tone string `json:"tone"`
	// Icon draws the trigger as an item's icon act (a 28px glyph, the trigger
	// its tip) rather than as the act's named control, and lays the confirm
	// into the item's own row, so the question drops full-width under the item
	// it asks about. For an act on one of a set's items: removing a key.
	Icon string `json:"icon,omitempty"`
	// subject is the item an icon act acts on, named after the trigger for
	// anyone who hears the control rather than sees its row.
	subject string
}

// Confirm tones: the closed set a Tone resolves to.
const (
	ToneDanger  = "danger"
	ToneCaution = "caution"
)

func (*Confirm) isWidget() {}

func (*Confirm) children() []Widget { return nil }

// confirmView is the confirm template's model: a render-unique id (so several
// confirms on a page never share checkbox state) plus the resolved labels.
type confirmView struct {
	ID                                             string
	Trigger, Title, Message, Confirm, Cancel, Tone string
	Icon, Subject                                  string
	RequirePassword                                bool
}

func (c *Confirm) renderInto(r *Renderer, out io.Writer, _ string) error {
	// Unset, the answer repeats the act it confirms — the question and the
	// button name the same act ("Delete rule"), never a bare "Confirm". The
	// trigger has been through the catalog already, so it reads in the
	// reader's language.
	confirm := c.Confirm
	if confirm == "" {
		confirm = c.Trigger
	}
	if confirm == "" {
		confirm = r.tr("Confirm")
	}
	cancel := c.Cancel
	if cancel == "" {
		cancel = r.tr("Cancel")
	}
	tone := ToneDanger
	if c.Tone == ToneCaution {
		tone = ToneCaution
	}
	return r.execute(out, "confirm.html.tmpl", confirmView{
		ID:              fmt.Sprintf("verso-confirm-%d", r.seq.cfm.Add(1)),
		Tone:            tone,
		Trigger:         c.Trigger,
		Title:           c.Title,
		Message:         c.Message,
		Confirm:         confirm,
		Cancel:          cancel,
		Icon:            c.Icon,
		Subject:         c.subject,
		RequirePassword: c.RequirePassword,
	})
}

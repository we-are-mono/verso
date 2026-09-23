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
	Confirm         string `json:"confirm"`          // confirm-button label (default "Confirm")
	Cancel          string `json:"cancel"`           // cancel label (default "Cancel")
	RequirePassword bool   `json:"require_password"` // ask for the current administrator password
	// Tone is what the act costs. "caution" is disruptive but wanted — installing
	// firmware — and asks in marigold, so it steadies rather than scares. Anything
	// else is danger: the act destroys something, and asks in crimson.
	Tone string `json:"tone"`
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
	RequirePassword                                bool
}

func (c *Confirm) renderInto(r *Renderer, out io.Writer, _ string) error {
	confirm := c.Confirm
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
		RequirePassword: c.RequirePassword,
	})
}

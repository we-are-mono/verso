// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"io"
)

// Confirm guards a destructive action with an inline confirmation: a danger button
// that, when pressed, swaps in place for a short "are you sure?" with a cancel and a
// confirm — no separate dialog, no leaving the spot. It is pure CSS (ADR-005 §7): a
// hidden checkbox flips between the two states, so it needs no JavaScript.
type Confirm struct {
	Trigger string `json:"trigger"` // the danger button's label, e.g. "Remove device"
	Message string `json:"message"` // the confirmation prompt
	Confirm string `json:"confirm"` // confirm-button label (default "Confirm")
	Cancel  string `json:"cancel"`  // cancel label (default "Cancel")
}

func (*Confirm) isWidget() {}

// confirmView is the confirm template's model: a render-unique id (so several
// confirms on a page never share checkbox state) plus the resolved labels.
type confirmView struct {
	ID                       string
	Trigger, Message, Confirm, Cancel string
}

func (c *Confirm) renderInto(r *Renderer, out io.Writer, _ string) error {
	confirm := c.Confirm
	if confirm == "" {
		confirm = "Confirm"
	}
	cancel := c.Cancel
	if cancel == "" {
		cancel = "Cancel"
	}
	return r.execute(out, "confirm.html.tmpl", confirmView{
		ID:      fmt.Sprintf("verso-confirm-%d", r.cfmSeq.Add(1)),
		Trigger: c.Trigger, Message: c.Message, Confirm: confirm, Cancel: cancel,
	})
}

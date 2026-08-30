// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Modal is an overlay dialog opened by a trigger button. It is a shell-owned
// behavioural widget (ADR-005 §7) realized with Alpine (ADR-004): the plugin
// declares the intent — a trigger label, a title, and the dialog's contents — and
// the shell renders the markup plus the open/close/focus behaviour. Plugins ship
// no JS; the shell owns every pixel and every interaction.
type Modal struct {
	Trigger      string   // label of the button that opens the dialog
	TriggerStyle string   // "" (default solid button) | "add" (full-width dashed add affordance)
	Preview      bool     // render the open window in-flow without trigger/backdrop/behaviour (styleguide use)
	Variant      string   // "" (standard composed modal) | "danger" (destructive confirmation)
	Title        string   // dialog heading
	Body         string   // destructive confirmation explanation
	Confirm      string   // destructive confirmation action label
	Cancel       string   // destructive confirmation cancel label
	Children     []Widget // dialog body
}

func (*Modal) isWidget() {}

// UnmarshalJSON decodes a modal's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing.
func (m *Modal) UnmarshalJSON(data []byte) error {
	var raw struct {
		Trigger      string            `json:"trigger"`
		TriggerStyle string            `json:"trigger_style"`
		Preview      bool              `json:"preview"`
		Variant      string            `json:"variant"`
		Title        string            `json:"title"`
		Body         string            `json:"body"`
		Confirm      string            `json:"confirm"`
		Cancel       string            `json:"cancel"`
		Children     []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Trigger = raw.Trigger
	m.TriggerStyle = raw.TriggerStyle
	m.Preview = raw.Preview
	m.Variant = raw.Variant
	m.Title = raw.Title
	m.Body = raw.Body
	m.Confirm = raw.Confirm
	m.Cancel = raw.Cancel
	m.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("modal child %d: %w", i, err)
		}
		m.Children = append(m.Children, w)
	}
	return nil
}

// modalView is the modal template's model: the trigger label and style, title, and
// the dialog body already rendered to trusted HTML.
type modalView struct {
	Trigger      string
	TriggerStyle string
	Preview      bool
	Variant      string
	Title        string
	Body         string
	Confirm      string
	Cancel       string
	Children     []template.HTML
}

// renderInto renders each child through the renderer, so the dialog body composes
// the closed widget set. The template carries the Alpine directives; the behaviour
// (open/close/focus) is the shell's verso.js (ADR-004).
func (m *Modal) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(m.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "modal.html.tmpl", modalView{
		Trigger: m.Trigger, TriggerStyle: m.TriggerStyle, Preview: m.Preview, Variant: m.Variant,
		Title: m.Title, Body: m.Body, Confirm: m.Confirm, Cancel: m.Cancel, Children: children,
	})
}

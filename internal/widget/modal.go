// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Modal is an overlay dialog opened by a trigger button. It is a shell-owned
// behavioural widget (ADR-005 §7) realized with Alpine (ADR-004): the plugin
// declares the intent — a trigger label, a title, and the dialog's contents — and
// the shell renders the markup plus the open/close/focus behaviour. Plugins ship
// no JS; the shell owns every pixel and every interaction.
type Modal struct {
	Trigger      string // label of the button that opens the dialog
	TriggerIcon  string
	TriggerStyle string // "" (default solid button) | "secondary" | "add"
	Open         bool   // open immediately (server-rendered verified/error state)
	BusyTitle    string // optional submit-time progress state
	BusyBody     string
	Preview      bool     // render the open window in-flow without trigger/backdrop/behaviour (styleguide use)
	Title        string   // dialog heading
	Children     []Widget // dialog body
}

func (*Modal) isWidget() {}

func (m *Modal) children() []Widget { return m.Children }

// UnmarshalJSON decodes a modal's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing.
func (m *Modal) UnmarshalJSON(data []byte) error {
	var raw struct {
		Trigger      string            `json:"trigger"`
		TriggerIcon  string            `json:"trigger_icon"`
		TriggerStyle string            `json:"trigger_style"`
		Open         bool              `json:"open"`
		BusyTitle    string            `json:"busy_title"`
		BusyBody     string            `json:"busy_body"`
		Preview      bool              `json:"preview"`
		Title        string            `json:"title"`
		Children     []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Trigger = raw.Trigger
	m.TriggerIcon = raw.TriggerIcon
	m.TriggerStyle = raw.TriggerStyle
	m.Open = raw.Open
	m.BusyTitle = raw.BusyTitle
	m.BusyBody = raw.BusyBody
	m.Preview = raw.Preview
	m.Title = raw.Title
	children, err := decodeChildren(raw.Children, "modal child")
	if err != nil {
		return err
	}
	m.Children = children
	return nil
}

// modalView is the modal template's model: the trigger label and style, title, and
// the dialog body already rendered to trusted HTML.
type modalView struct {
	Trigger      string
	TriggerIcon  string
	TriggerStyle string
	Open         bool
	BusyTitle    string
	BusyBody     string
	Preview      bool
	Title        string
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
		Trigger: m.Trigger, TriggerIcon: m.TriggerIcon, TriggerStyle: m.TriggerStyle, Open: m.Open,
		BusyTitle: m.BusyTitle, BusyBody: m.BusyBody, Preview: m.Preview,
		Title: m.Title, Children: children,
	})
}

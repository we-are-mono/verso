// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
)

// Modal is an overlay dialog opened by a trigger button. It is a shell-owned
// behavioural widget (ADR-005 §7) realized with Alpine (ADR-004): the plugin
// declares the intent — a trigger label, a title, and the dialog's contents — and
// the shell renders the markup plus the open/close/focus behaviour. Plugins ship
// no JS; the shell owns every pixel and every interaction.
type Modal struct {
	Trigger      string `json:"trigger"` // label of the button that opens the dialog
	TriggerIcon  string `json:"trigger_icon"`
	TriggerStyle string `json:"trigger_style"` // "" (default solid button) | "secondary" | "link"
	Open         bool   `json:"open"`          // open immediately (server-rendered verified/error state)
	BusyTitle    string `json:"busy_title"`    // optional submit-time progress state
	BusyBody     string `json:"busy_body"`
	// BusyBodyVerbatim marks a busy line the shell composed around data after
	// translating it (a board's model in a sentence); the walk leaves it.
	BusyBodyVerbatim bool    `json:"-"`
	Title            string  `json:"title"`    // dialog heading
	Children         Widgets `json:"children"` // dialog body
	// Steps names the steps a dialog walks through ("Choose", "Verify",
	// "Install"), and Step is the one in hand (0-based). The dialog says where
	// it is under its title; the page's script moves it on as an upload goes.
	Steps []string `json:"steps"`
	Step  int      `json:"step"`
}

func (*Modal) isWidget() {}

func (m *Modal) children() []Widget { return m.Children }

func (m *Modal) prune(keep func(Widget) bool) { m.Children = pruneList(m.Children, keep) }

// modalView is the modal template's model: the trigger label and style, title, and
// the dialog body already rendered to trusted HTML.
type modalView struct {
	Trigger      string
	TriggerIcon  string
	TriggerStyle string
	Open         bool
	BusyTitle    string
	BusyBody     string
	Title        string
	Children     []template.HTML
	Steps        []modalStep
}

// modalStep is one step of the dialog's line, with where it stands: "done",
// "current" or "ahead".
type modalStep struct {
	Label, State string
}

func (m *Modal) view(r *Renderer, csrf string) (modalView, error) {
	children, err := r.renderChildren(m.Children, csrf)
	if err != nil {
		return modalView{}, err
	}
	steps := make([]modalStep, len(m.Steps))
	for i, label := range m.Steps {
		state := "ahead"
		switch {
		case i < m.Step:
			state = "done"
		case i == m.Step:
			state = "current"
		}
		steps[i] = modalStep{Label: label, State: state}
	}
	return modalView{
		Trigger: m.Trigger, TriggerIcon: m.TriggerIcon, TriggerStyle: m.TriggerStyle, Open: m.Open,
		BusyTitle: m.BusyTitle, BusyBody: m.BusyBody,
		Title: m.Title, Children: children, Steps: steps,
	}, nil
}

// renderInto renders each child through the renderer, so the dialog body composes
// the closed widget set. The template carries the Alpine directives; the behaviour
// (open/close/focus) is the shell's verso.js (ADR-004).
func (m *Modal) renderInto(r *Renderer, out io.Writer, csrf string) error {
	v, err := m.view(r, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "modal.html.tmpl", v)
}

// RenderModalContents renders what an open dialog holds — its title, its steps
// and its body — without its trigger or its frame. It is the answer to a dialog
// posting its own upload: the dialog on screen swaps what it holds for this,
// rather than the page being drawn again around it.
func (r *Renderer) RenderModalContents(out io.Writer, m *Modal, csrfToken, lang string, t func(string) string) error {
	if t != nil {
		translateSchema(m, t)
	}
	pass := &Renderer{tmpl: r.setFor(lang), t: t, md: r.md, seq: r.seq}
	v, err := m.view(pass, csrfToken)
	if err != nil {
		return err
	}
	return pass.execute(out, "modal.standard.contents", v)
}

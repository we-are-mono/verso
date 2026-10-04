// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// Conditional is a behavioural widget: one field-set shown when its controlling
// toggle is on, with an optional alternate field-set shown when it is off
// (ADR-005 §7). The plugin declares the branches and the shell realizes them in
// pure CSS (no JavaScript, no round-trip), because show/hide has no state to
// persist. The toggle posts its own value, so the plugin can read it when
// interpreting a save.
type Conditional struct {
	Name      string  `json:"name"`      // form field name of the controlling toggle
	Label     string  `json:"label"`     // the toggle's label
	Checked   bool    `json:"checked"`   // whether the toggle starts on
	Fields    Widgets `json:"fields"`    // the field-set revealed when the toggle is on
	Otherwise Widgets `json:"otherwise"` // optional field-set revealed when the toggle is off
	// Key is the option the toggle writes, worn as the mono chip a field's label
	// wears — the toggle is a form row like any other, so it reads like one.
	Key string `json:"key"`
	// Help is the line under the toggle: what turning it on means, in the place
	// a field's helper line sits.
	Help string `json:"help"`
	// Staged is the shell's word that the toggle's option waits to be applied.
	// The gate names only its key; the form or section around it says where.
	Staged bool `json:"-"`
}

func (*Conditional) isWidget() {}

// LabelView is the gate's left column — the same shape a field's and a switch's
// is. A gate that looked unlike the fields it gates would read as a different
// kind of thing, and it is not: it is the setting the rest depend on.
func (c *Conditional) LabelView() fieldLabel {
	tip := (&Field{Name: c.Name, Key: c.Key, Help: c.Help}).TipView()
	return fieldLabel{
		For: c.Name, Label: c.Label, Key: c.Key,
		Explained: c.Help != "", Tip: tip, Staged: c.Staged,
	}
}

func (c *Conditional) children() []Widget {
	return append(append([]Widget{}, c.Fields...), c.Otherwise...)
}

func (c *Conditional) prune(keep func(Widget) bool) {
	c.Fields = pruneList(c.Fields, keep)
	c.Otherwise = pruneList(c.Otherwise, keep)
}

// conditionalView is the conditional template's model: the gate's row and the
// gated fields, already rendered to trusted HTML.
type conditionalView struct {
	Name, Label string
	Gate        template.HTML
	Fields      []template.HTML
	Otherwise   []template.HTML
}

// control is the gate's checkbox. The gate's label points at it by id, so it
// carries no name of its own; an explained gate is described by its tip.
func (c *Conditional) control() switchControl {
	ctl := switchControl{Name: c.Name, On: c.Checked}
	if c.Help != "" {
		ctl.Described = c.Name + "-tip"
	}
	return ctl
}

// renderInto renders the gate as a setting's row and the gated field-set through
// the renderer, then hands the template both. Visibility is pure CSS (ADR-005
// §7): the shell owns the toggle and the show/hide, the plugin only declared the
// intent. The block around the gate tracks the change, so the gate's row does not.
func (c *Conditional) renderInto(r *Renderer, out io.Writer, csrf string) error {
	var gate strings.Builder
	if err := r.renderFrame(&gate, "switch.row", c.control(), fieldFrame{
		Label: c.LabelView(), Toggle: true, Class: "verso-conditional-gate py-0",
	}); err != nil {
		return err
	}
	fields, err := r.renderChildren(c.Fields, csrf)
	if err != nil {
		return err
	}
	otherwise, err := r.renderChildren(c.Otherwise, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "conditional.html.tmpl", conditionalView{
		Name: c.Name, Label: c.Label, Gate: template.HTML(gate.String()),
		Fields: fields, Otherwise: otherwise,
	})
}

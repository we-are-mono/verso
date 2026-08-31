// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Conditional is a behavioural widget: one field-set shown when its controlling
// toggle is on, with an optional alternate field-set shown when it is off
// (ADR-005 §7). The plugin declares the branches and the shell realizes them in
// pure CSS (no JavaScript, no round-trip), because show/hide has no state to
// persist. The toggle posts its own value, so the plugin can read it when
// interpreting a save.
type Conditional struct {
	Name      string   // form field name of the controlling toggle
	Label     string   // the toggle's label
	Checked   bool     // whether the toggle starts on
	Fields    []Widget // the field-set revealed when the toggle is on
	Otherwise []Widget // optional field-set revealed when the toggle is off
}

func (*Conditional) isWidget() {}

func (c *Conditional) children() []Widget {
	return append(append([]Widget{}, c.Fields...), c.Otherwise...)
}

func (c *Conditional) prune(keep func(Widget) bool) {
	c.Fields = pruneList(c.Fields, keep)
	c.Otherwise = pruneList(c.Otherwise, keep)
}

// UnmarshalJSON decodes the gated fields recursively through Decode, so an unknown
// field type fails loudly rather than vanishing.
func (c *Conditional) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name      string            `json:"name"`
		Label     string            `json:"label"`
		Checked   bool              `json:"checked"`
		Fields    []json.RawMessage `json:"fields"`
		Otherwise []json.RawMessage `json:"otherwise"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Name = raw.Name
	c.Label = raw.Label
	c.Checked = raw.Checked
	var err error
	if c.Fields, err = decodeChildren(raw.Fields, "conditional field"); err != nil {
		return err
	}
	if c.Otherwise, err = decodeChildren(raw.Otherwise, "conditional otherwise field"); err != nil {
		return err
	}
	return nil
}

// conditionalView is the conditional template's model: the toggle plus the gated
// fields, already rendered to trusted HTML.
type conditionalView struct {
	Name, Label string
	On          bool
	Fields      []template.HTML
	Otherwise   []template.HTML
}

// renderInto renders the gated field-set through the renderer, then hands the
// template the controlling toggle. Visibility is pure CSS (ADR-005 §7): the shell
// owns the toggle and the show/hide, the plugin only declared the intent.
func (c *Conditional) renderInto(r *Renderer, out io.Writer, csrf string) error {
	fields, err := r.renderChildren(c.Fields, csrf)
	if err != nil {
		return err
	}
	otherwise, err := r.renderChildren(c.Otherwise, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "conditional.html.tmpl", conditionalView{
		Name: c.Name, Label: c.Label, On: c.Checked,
		Fields: fields, Otherwise: otherwise,
	})
}

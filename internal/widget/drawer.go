// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Drawer reveals detail for one item without leaving the page: tapping its trigger
// (typically the item's row) slides a panel in from the edge. It is the natural
// companion to a list — the row shows the summary, the drawer the full story. Like
// modal it is shell-owned behaviour (ADR-005 §7) realized with Alpine (ADR-004),
// and it reuses the very same open/close/focus-trap component; only the layout
// differs. The plugin declares the trigger, a title, and the panel body; the shell
// owns every pixel and interaction. Plugins ship no JS.
type Drawer struct {
	Title    string   // panel heading
	Trigger  []Widget // what opens the drawer (e.g. a row)
	Children []Widget // panel body
	// Size widens the panel: "" (the reading width) | "wide" — for detail
	// views that carry tables beside prose.
	Size string `json:"size,omitempty"`
	// Style dresses the trigger: "" wraps it as a framed card button; "bare"
	// leaves it an unstyled block with the row hover tint — for triggers that
	// live inside a hairline-divided list.
	Style string `json:"style,omitempty"`
}

func (*Drawer) isWidget() {}

// UnmarshalJSON decodes the trigger and body recursively through Decode, so an
// unknown child type fails loudly rather than vanishing (as modal does).
func (d *Drawer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title    string            `json:"title"`
		Size     string            `json:"size"`
		Style    string            `json:"style"`
		Trigger  []json.RawMessage `json:"trigger"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Title = raw.Title
	d.Size = raw.Size
	d.Style = raw.Style
	d.Trigger = make([]Widget, 0, len(raw.Trigger))
	for i, rc := range raw.Trigger {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("drawer trigger %d: %w", i, err)
		}
		d.Trigger = append(d.Trigger, w)
	}
	d.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("drawer child %d: %w", i, err)
		}
		d.Children = append(d.Children, w)
	}
	return nil
}

// drawerView is the drawer template's model: the trigger and body already rendered
// to trusted HTML, plus the panel title and its variants.
type drawerView struct {
	Title    string
	Wide     bool
	Bare     bool
	Trigger  template.HTML
	Children []template.HTML
}

// renderInto renders the trigger and the panel body through the renderer, then hands
// the template the slide-in chrome. The behaviour is the shell's modal component
// (ADR-004); the plugin supplied only the trigger, title, and body (ADR-005 §7).
func (d *Drawer) renderInto(r *Renderer, out io.Writer, csrf string) error {
	trigger, err := r.renderChildren(d.Trigger, csrf)
	if err != nil {
		return err
	}
	children, err := r.renderChildren(d.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "drawer.html.tmpl", drawerView{
		Title: d.Title, Wide: d.Size == "wide", Bare: d.Style == "bare",
		Trigger: joinHTML(trigger), Children: children,
	})
}

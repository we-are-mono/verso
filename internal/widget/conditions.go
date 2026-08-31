// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Conditions is a typed optional-field builder. A plugin declares the complete
// catalogue and which entries are currently active; the shell owns the add/remove
// interaction and renders each entry from ordinary widgets. This keeps complex
// editors complete without presenting a wall of empty controls.
type Conditions struct {
	Label string          `json:"label"`
	Help  string          `json:"help,omitempty"`
	Items []ConditionItem `json:"items"`
}

// ConditionItem is one uniquely keyed optional condition. Active items render in
// the form immediately; inactive items remain available from the picker. Children
// are normal widgets, so validation and POST semantics stay unchanged.
type ConditionItem struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Active   bool     `json:"active,omitempty"`
	Children []Widget `json:"-"`
}

func (*Conditions) isWidget() {}

func (c *Conditions) children() []Widget {
	var out []Widget
	for i := range c.Items {
		out = append(out, c.Items[i].Children...)
	}
	return out
}

func (c *Conditions) prune(keep func(Widget) bool) {
	for i := range c.Items {
		c.Items[i].Children = pruneList(c.Items[i].Children, keep)
	}
}

func (c *Conditions) UnmarshalJSON(data []byte) error {
	var raw struct {
		Label string `json:"label"`
		Help  string `json:"help"`
		Items []struct {
			Key      string            `json:"key"`
			Label    string            `json:"label"`
			Help     string            `json:"help"`
			Active   bool              `json:"active"`
			Children []json.RawMessage `json:"children"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Label, c.Help = raw.Label, raw.Help
	c.Items = make([]ConditionItem, 0, len(raw.Items))
	seen := make(map[string]bool, len(raw.Items))
	for i, item := range raw.Items {
		if item.Key == "" {
			return fmt.Errorf("condition item %d: missing key", i)
		}
		if seen[item.Key] {
			return fmt.Errorf("condition item %d: duplicate key %q", i, item.Key)
		}
		seen[item.Key] = true
		children, err := decodeChildren(item.Children, fmt.Sprintf("condition item %d child", i))
		if err != nil {
			return err
		}
		c.Items = append(c.Items, ConditionItem{
			Key: item.Key, Label: item.Label, Help: item.Help, Active: item.Active, Children: children,
		})
	}
	return nil
}

type conditionOptionView struct {
	Key, Label string
	Active     bool
}

type conditionsView struct {
	Label, Help string
	Active      []template.HTML
	Templates   []template.HTML
	Options     []conditionOptionView
}

type conditionItemView struct {
	Key, Label, Help string
	Children         []template.HTML
}

func (c *Conditions) renderInto(r *Renderer, out io.Writer, csrf string) error {
	view := conditionsView{Label: c.Label, Help: c.Help}
	for _, item := range c.Items {
		children, err := r.renderChildren(item.Children, csrf)
		if err != nil {
			return err
		}
		iv := conditionItemView{Key: item.Key, Label: item.Label, Help: item.Help, Children: children}
		var b strings.Builder
		if err := r.execute(&b, "conditions.item", iv); err != nil {
			return err
		}
		html := template.HTML(b.String())
		if item.Active {
			view.Active = append(view.Active, html)
		}
		view.Templates = append(view.Templates, html)
		view.Options = append(view.Options, conditionOptionView{Key: item.Key, Label: item.Label, Active: item.Active})
	}
	return r.execute(out, "conditions.html.tmpl", view)
}

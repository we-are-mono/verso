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
	Key   string `json:"key"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
	// Group is the heading this entry sits under in the picker. A catalogue long
	// enough to need searching is long enough to need sorting into kinds, and a
	// flat list of everything a rule could match on is the wall of controls the
	// picker exists to avoid. Entries naming no group are listed first, ungrouped.
	Group string `json:"group,omitempty"`
	// Hint is what kind of value the condition takes — "one host or a subnet",
	// "names or numbers" — shown at the picker row's trailing edge. It is an
	// example, not an explanation: Help says what the condition is for and belongs
	// to the row once it is added, while this says what you would type.
	Hint     string   `json:"hint,omitempty"`
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
			Group    string            `json:"group"`
			Hint     string            `json:"hint"`
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
			Key: item.Key, Label: item.Label, Help: item.Help, Group: item.Group,
			Hint: item.Hint, Active: item.Active, Children: children,
		})
	}
	return nil
}

type conditionOptionView struct {
	Key, Label, Hint string
	Active           bool
}

// conditionGroupView is one heading in the picker and the entries under it, in
// the order the catalogue declared them.
type conditionGroupView struct {
	Name  string
	Items []conditionOptionView
}

type conditionsView struct {
	Label, Help string
	Active      []template.HTML
	Templates   []template.HTML
	Options     []conditionOptionView
	Groups      []conditionGroupView
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
		view.Options = append(view.Options, conditionOptionView{
			Key: item.Key, Label: item.Label, Hint: item.Hint, Active: item.Active,
		})
	}
	view.Groups = groupConditions(c.Items, view.Options)
	return r.execute(out, "conditions.html.tmpl", view)
}

// groupConditions sorts the catalogue into the picker's headings, keeping both the
// groups and the entries inside them in the order the plugin declared them — the
// catalogue's order is an editorial judgement about what someone reaches for
// first, and alphabetising it would throw that away.
func groupConditions(items []ConditionItem, options []conditionOptionView) []conditionGroupView {
	var groups []conditionGroupView
	at := make(map[string]int, len(items))
	for i, item := range items {
		index, seen := at[item.Group]
		if !seen {
			index = len(groups)
			at[item.Group] = index
			groups = append(groups, conditionGroupView{Name: item.Group})
		}
		groups[index].Items = append(groups[index].Items, options[i])
	}
	return groups
}

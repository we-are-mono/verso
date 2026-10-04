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
	Hint     string  `json:"hint,omitempty"`
	Active   bool    `json:"active,omitempty"`
	Children Widgets `json:"children"`
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

// UnmarshalJSON refuses an item without a key, or with a key another item
// already has: the key is what the picker adds and removes an item by.
func (c *Conditions) UnmarshalJSON(data []byte) error {
	type plain Conditions
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}
	seen := make(map[string]bool, len(c.Items))
	for i, item := range c.Items {
		if item.Key == "" {
			return fmt.Errorf("condition item %d: missing key", i)
		}
		if seen[item.Key] {
			return fmt.Errorf("condition item %d: duplicate key %q", i, item.Key)
		}
		seen[item.Key] = true
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

// conditionItemView is one added condition: its name line, labelled as every
// row in the form is (Name), whether it is a single control drawn straight
// under that line (Single), and its controls.
type conditionItemView struct {
	Key, Label string
	Name       fieldLabel
	Single     bool
	Children   []template.HTML
}

// conditionName is a condition's name line: what it is called, the key it is
// known by in the picker, and what it is for, raised on the name. A condition
// of one control draws that control's staged mark here, because the control's
// own label line is not drawn.
func conditionName(item ConditionItem, single bool) fieldLabel {
	id := "condition-" + item.Key
	name := fieldLabel{
		For: id, Group: true, Label: item.Label, Key: item.Key,
		Explained: item.Help != "", Tip: TipView{ID: id + "-tip", Tip: item.Help, Footer: item.Key},
	}
	if single {
		switch child := item.Children[0].(type) {
		case *Field:
			name.Staged = child.Staged
		case *List:
			name.Staged = child.Staged
		}
	}
	return name
}

// singleControl reports whether a condition is one control — a field or a
// list — whose own label would only repeat the condition's name.
func singleControl(children []Widget) bool {
	if len(children) != 1 {
		return false
	}
	switch children[0].(type) {
	case *Field, *List:
		return true
	}
	return false
}

func (c *Conditions) renderInto(r *Renderer, out io.Writer, csrf string) error {
	view := conditionsView{Label: c.Label, Help: c.Help}
	for _, item := range c.Items {
		children, err := r.renderChildren(item.Children, csrf)
		if err != nil {
			return err
		}
		single := singleControl(item.Children)
		iv := conditionItemView{Key: item.Key, Label: item.Label, Name: conditionName(item, single), Single: single, Children: children}
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

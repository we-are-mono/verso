// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Tabs is a segmented switch between a few panels — the intent split at the top
// of a page (e.g. "My devices" vs "Route through a provider"). It is pure CSS
// (ADR-005 §7): a radio group drives which panel shows, so it needs no JavaScript
// and gets native arrow-key navigation for free. Plugins declare the tabs and
// their contents; the shell owns the chrome and the switching.
type Tabs struct {
	Tabs []Tab
}

// Tab is one labelled panel: an optional leading icon, a label, and the widget
// subtree shown when the tab is active.
type Tab struct {
	Label    string
	Icon     string
	Children []Widget
}

func (*Tabs) isWidget() {}

// UnmarshalJSON decodes each tab's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing (as modal/card do).
func (t *Tabs) UnmarshalJSON(data []byte) error {
	var raw struct {
		Tabs []struct {
			Label    string            `json:"label"`
			Icon     string            `json:"icon"`
			Children []json.RawMessage `json:"children"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.Tabs = make([]Tab, 0, len(raw.Tabs))
	for i, rt := range raw.Tabs {
		tab := Tab{Label: rt.Label, Icon: rt.Icon, Children: make([]Widget, 0, len(rt.Children))}
		for j, rc := range rt.Children {
			w, err := Decode(rc)
			if err != nil {
				return fmt.Errorf("tab %d child %d: %w", i, j, err)
			}
			tab.Children = append(tab.Children, w)
		}
		t.Tabs = append(t.Tabs, tab)
	}
	return nil
}

// tabView is one rendered tab: its label, icon, first-tab flag (which radio is
// checked), and its panel body already rendered to trusted HTML.
type tabView struct {
	Label, Icon string
	First       bool
	Body        template.HTML
}

// tabsView is the tabs template's model: a group name unique to this render (so
// two tab groups on one page never share radio state) and the rendered tabs.
type tabsView struct {
	Group string
	Tabs  []tabView
}

// renderInto renders each tab's subtree through the renderer, then hands the
// template a radio group whose :checked state (pure CSS) switches panels. The plugin
// declared the tabs; every affordance and the switching is the shell's (ADR-005 §7).
func (t *Tabs) renderInto(r *Renderer, out io.Writer, csrf string) error {
	group := fmt.Sprintf("verso-tabs-%d", r.seq.tab.Add(1))
	tabs := make([]tabView, 0, len(t.Tabs))
	for i, tab := range t.Tabs {
		body, err := r.renderChildren(tab.Children, csrf)
		if err != nil {
			return err
		}
		tabs = append(tabs, tabView{Label: tab.Label, Icon: tab.Icon, First: i == 0, Body: joinHTML(body)})
	}
	return r.execute(out, "tabs.html.tmpl", tabsView{Group: group, Tabs: tabs})
}

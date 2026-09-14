// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Drawer reveals detail for one item without leaving the page: tapping its trigger
// (typically the item's row) slides a panel in from the edge. It is the natural
// companion to a list — the row shows the summary, the drawer the full story. An
// object lives in its drawer (ADR-005 §8): the panel holds its facts, its form
// and its acts, and a Save inside it stages like any other write. Like
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
	// live inside a hairline-divided list; "button" wears the primary action
	// button — for an affordance whose detail panel is the drawer.
	Style string `json:"style,omitempty"`
	// Deprecated: Dot and Tag are accepted for wire compatibility. Drawer
	// headings contain only the title; status belongs in the body.
	Dot string `json:"dot,omitempty"`
	Tag string `json:"tag,omitempty"`
}

func (*Drawer) isWidget() {}

func (d *Drawer) children() []Widget {
	return append(append([]Widget{}, d.Trigger...), d.Children...)
}

func (d *Drawer) prune(keep func(Widget) bool) {
	d.Trigger = pruneList(d.Trigger, keep)
	d.Children = pruneList(d.Children, keep)
}

// UnmarshalJSON decodes the trigger and body recursively through Decode, so an
// unknown child type fails loudly rather than vanishing (as modal does).
func (d *Drawer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title    string            `json:"title"`
		Size     string            `json:"size"`
		Style    string            `json:"style"`
		Dot      string            `json:"dot"`
		Tag      string            `json:"tag"`
		Trigger  []json.RawMessage `json:"trigger"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Title = raw.Title
	d.Size = raw.Size
	d.Style = raw.Style
	d.Dot = raw.Dot
	d.Tag = raw.Tag
	trigger, err := decodeChildren(raw.Trigger, "drawer trigger")
	if err != nil {
		return err
	}
	d.Trigger = trigger
	children, err := decodeChildren(raw.Children, "drawer child")
	if err != nil {
		return err
	}
	d.Children = children
	return nil
}

// Flash is a one-shot outcome as the flash slot draws it — a confirmation, a
// plugin envelope's notice — toned by the tone vocabulary. The page carries one
// at the top of its content; a panel answering its own submission carries one
// at the top of its body, because the page around it is not re-rendered to
// show it. An empty Message draws nothing.
type Flash struct {
	Variant string
	Message string
}

// drawerPanelView is the slide-in panel's model — one shape for the drawer
// widget and a table row's drawer, so the two render through one template
// (verso-drawer-panel) and cannot drift apart.
type drawerPanelView struct {
	Choices bool
	Form    bool
	Title   string
	Closed  string
	Tabs    []DrawerTab
	Wide    bool
	Flash   Flash
	Body    []template.HTML
}

// renderPanel draws a row drawer into the one panel view every open panel is
// drawn from — a row's, the bar's, and the answer to a frame asking for its
// contents alone. An open panel is a place, its address in the address bar, so
// its forms post back to that address and into the frame that holds them
// rather than navigating the page already on screen behind it.
func (r *Renderer) renderPanel(d *RowDrawer, csrf string, flash Flash) (drawerPanelView, error) {
	for _, child := range d.Children {
		PostIntoFrame(child, FramePanel, "")
	}
	body, err := r.renderChildren(d.Children, csrf)
	if err != nil {
		return drawerPanelView{}, err
	}
	panel := drawerPanel(d, body)
	panel.Flash = flash
	return panel, nil
}

// drawerPanel builds the title, tabs, and body. Legacy heading decorations are
// deliberately omitted. Tab addresses pass the same URL policy as other links.
func drawerPanel(d *RowDrawer, body []template.HTML) drawerPanelView {
	panel := drawerPanelView{
		Title:  d.Title,
		Closed: SafeHref(d.Closed),
		Wide:   d.Size == "wide", Choices: d.Size == "choices", Form: d.Size == "form", Body: body,
	}
	panel.Tabs = make([]DrawerTab, 0, len(d.Tabs))
	for _, tab := range d.Tabs {
		tab.Href = SafeHref(tab.Href)
		panel.Tabs = append(panel.Tabs, tab)
	}
	return panel
}

// drawerView is the drawer template's model: the rendered trigger and its
// dress, plus the shared panel.
type drawerView struct {
	Bare    bool
	Button  bool
	Trigger template.HTML
	Panel   drawerPanelView
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
		Bare: d.Style == "bare", Button: d.Style == "button",
		Trigger: joinHTML(trigger),
		Panel: drawerPanelView{
			Title: d.Title, Wide: d.Size == "wide", Body: children,
		},
	})
}

// RenderOpenPanelWithToken renders only the contents of whichever panel the tree
// says is open — the nameplate, the strip of readings, and the body — and reports
// whether it found one. It is the answer to a request that is not asking for a
// page: the frame is already on screen and only what it holds is being replaced,
// so re-sending the listing around it would be the page rebuilding itself to
// change the part of it that moved.
//
// The plugin is not asked anything different. It already answers an address
// naming an open panel with exactly that panel open, and it answers the panel's
// own submission the same way; this only takes the panel out of the answer and
// leaves the rest. What the page would have said about that submission in its
// flash slot rides in as flash, at the top of the body, since the page is not
// being drawn to say it.
func (r *Renderer) RenderOpenPanelWithToken(out io.Writer, w Widget, csrfToken, lang string, t func(string) string, flash Flash) (bool, error) {
	if t != nil {
		translateSchema(w, t)
	}
	pass := &Renderer{tmpl: r.setFor(lang), t: t, md: r.md, seq: r.seq}
	open := openPanel(w)
	if open == nil {
		return false, nil
	}
	panel, err := pass.renderPanel(open, csrfToken, flash)
	if err != nil {
		return false, err
	}
	return true, pass.execute(out, "verso-drawer-contents", panel)
}

// openPanel is the one panel a tree says is open: a row's, or the bar's own for
// an object that does not exist yet. A tree with none is a page that was asked
// for as a page.
func openPanel(w Widget) *RowDrawer {
	var found *RowDrawer
	Walk(w, func(n Widget) {
		if found != nil {
			return
		}
		switch n := n.(type) {
		case *ActionBar:
			if n.Drawer != nil && n.Drawer.Open {
				found = n.Drawer
			}
		case *Table:
			rows := n.Rows
			if n.Seam != nil {
				rows = append(append([]TableRow{}, rows...), n.Seam.Rows...)
			}
			for i := range rows {
				if rows[i].Drawer != nil && rows[i].Drawer.Open {
					found = rows[i].Drawer
					return
				}
			}
		}
	})
	return found
}

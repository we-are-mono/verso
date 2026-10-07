// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
)

// Flash is a one-shot outcome — a confirmation, a plugin envelope's notice —
// toned by the tone vocabulary. It is never drawn in the content: the page,
// and a panel answering its own submission, carry it in a template for the
// outcome layer at the top right of the viewport to say. An empty Message
// carries nothing.
type Flash struct {
	Variant string
	Message string
}

// drawerPanelView is the slide-in panel's model — one shape for the drawer
// widget and a table row's drawer, so the two render through one template
// (verso-drawer-panel) and cannot drift apart.
type drawerPanelView struct {
	Choices bool
	Title   string
	Closed  string
	Tabs    []DrawerTab
	Flash   Flash
	Body    []template.HTML
}

// drawer starts a panel's own section hierarchy. A drawer opened from a nested
// listing has the same headings as one rendered on its own after a form post.
// The trigger keeps the enclosing page's depth; only the body starts afresh.
func (r *Renderer) drawer() *Renderer {
	return &Renderer{tmpl: r.tmpl, t: r.t, md: r.md, seq: r.seq}
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
	body, err := r.drawer().renderChildren(d.Children, csrf)
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
		Title:   d.Title,
		Closed:  SafeHref(d.Closed),
		Choices: d.Size == "choices", Body: body,
	}
	panel.Tabs = make([]DrawerTab, 0, len(d.Tabs))
	for _, tab := range d.Tabs {
		tab.Href = SafeHref(tab.Href)
		panel.Tabs = append(panel.Tabs, tab)
	}
	return panel
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
// leaves the rest. What the page would have arrived with about that
// submission rides in as flash, carried for the notification to say, since
// the page is not being drawn around it.
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

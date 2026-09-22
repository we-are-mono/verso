// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// ActionBar is the row between a page's heading and its listing: which slice of
// the listing you are looking at, how to narrow it, and the one thing to do
// here. It is the listing's controls, not the page's — every part of it acts on
// the rows below and nothing else, which is why it sits with them rather than in
// the chrome.
//
// The parts read left to right in the order a person reaches for them. Tabs are
// the coarse cut, each carrying its own count so the cut is priced before it is
// made. Then, hard right, the fine cuts: a free-text Filter, a Select for the
// one dimension a listing is always sliced along, and Action — the single
// forward act, and the only denim on the bar.
//
// Everything here narrows what is already on screen, so the whole bar is
// client-side: nothing it does is a request, and nothing it does can fail.
type ActionBar struct {
	Style  string       `json:"style,omitempty"` // "interfaces": topology legend and one optional problem filter
	Tabs   []ActionTab  `json:"tabs,omitempty"`
	Filter string       `json:"filter,omitempty"` // the search field's placeholder; empty draws no field
	Select *ActionPick  `json:"select,omitempty"`
	Action *TableAction `json:"action,omitempty"`
	// Live is the label of the control that holds a running listing still. It
	// is not a narrowing, so the shell lifts it to the heading line with the
	// log's act (TakeHeadingAct): it reads Live, its spinner turning, while
	// events arrive and Paused while they are held, and pressing it is how that
	// changes. The shell owns the rest of that behaviour (verso-listing.js).
	Live string `json:"live,omitempty"`
	// Entity makes the act open a panel rather than leave the page: making a new
	// subject of the kind this listing holds is the same job as editing one, so
	// it happens in the same place. The address is the panel's, and the Action's
	// own Href is then only the fallback for a browser with no script.
	Entity string `json:"entity,omitempty"`
	// OpensPanel declares the act's own address a panel rather than a page, for a
	// listing whose plugin renders that panel itself: making one is editing one
	// that does not exist yet, so it opens in the same surface and the listing
	// stays where it is. The Href remains what a browser with no script follows.
	OpensPanel bool `json:"opens_panel,omitempty"`
	// Drawer is that same panel where the plugin draws it itself rather than the
	// shell assembling one: the act opens a blank object in the panel that edits
	// an existing one, which is the whole point — making one and changing one are
	// the same job and should not be two different screens. It carries its own
	// Open, so an address that asks for a new object arrives with the panel
	// already in front of the operator; the Action's Href stays the fallback.
	Drawer *RowDrawer `json:"drawer,omitempty"`
	// Heading renders the act alone, for the heading line: set by the shell
	// when the bar has nothing but its act (TakeHeadingAct), never by a plugin.
	Heading bool `json:"-"`
	// liveLog marks a live log's bar whose live control the shell lifted to
	// the heading line: it still sits over a log, not a table.
	liveLog bool
}

// overLog reports whether the bar sits over a live log rather than a table,
// which it wears unfilled.
func (a *ActionBar) overLog() bool { return a.Live != "" || a.liveLog }

// TakeHeadingAct lifts a page's acts onto its heading line. Every page keeps
// what it does there; the band under the heading is for narrowing the listing
// — the search, the cuts, the select — and nothing else. So a Stack whose
// first child is a band with a primary act gives that act up, with whatever it
// opens (its panel, its drawer, its entity), and the act is returned as a bar
// marked for the heading. A live log gives up its live control and its act,
// quiet or not, so the log's acts stand together as equals on the heading
// line. A band left with nothing to narrow goes from the body altogether. On a
// listing that is not live a quiet act takes something away rather than
// making something, so it stays with the narrowing; any other shape is left
// alone and nil is returned.
func TakeHeadingAct(w Widget) *ActionBar {
	stack, ok := w.(*Stack)
	if !ok || len(stack.Children) == 0 {
		return nil
	}
	bar, ok := stack.Children[0].(*ActionBar)
	if !ok || (bar.Live == "" && (bar.Action == nil || bar.Action.Quiet())) {
		return nil
	}
	act := &ActionBar{Live: bar.Live, Action: bar.Action, OpensPanel: bar.OpensPanel, Drawer: bar.Drawer, Entity: bar.Entity, Heading: true}
	bar.liveLog = bar.overLog()
	bar.Live, bar.Action, bar.OpensPanel, bar.Drawer, bar.Entity = "", nil, false, nil, ""
	if bar.Filter == "" && len(bar.Tabs) == 0 && bar.Select == nil && bar.Style == "" {
		stack.Children = stack.Children[1:]
	}
	return act
}

// ActionTab is one coarse cut of the listing, and what taking it would leave.
// Match is the value a row must carry under Key to survive the cut; an empty
// Match is the tab that cuts nothing.
type ActionTab struct {
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Match  string `json:"match,omitempty"`
	Active bool   `json:"active,omitempty"`
}

// ActionPick is the bar's one select: the dimension a listing is always sliced
// along (a devices listing by network, a log by severity). Key names the row
// attribute its options match against.
type ActionPick struct {
	Key     string         `json:"key"`
	Options []ActionOption `json:"options"`
}

// ActionOption is one value of that dimension. An empty Value is "all of them".
type ActionOption struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
}

func (*ActionBar) isWidget() {}

func (a *ActionBar) children() []Widget {
	if a.Drawer == nil {
		return nil
	}
	return a.Drawer.Children
}

func (a *ActionBar) prune(keep func(Widget) bool) {
	if a.Drawer != nil {
		a.Drawer.Children = pruneList(a.Drawer.Children, keep)
	}
}

// actionBarView is the bar plus, where the act opens one, the rendered panel it
// opens — the same panel model a row's drawer builds, so the two cannot drift.
type actionBarView struct {
	ActionBar
	HasPanel bool
	Open     bool
	Panel    drawerPanelView
	OverLog  bool // the bar sits over a live log, which it wears unfilled
}

func (a *ActionBar) renderInto(r *Renderer, out io.Writer, csrf string) error {
	v := actionBarView{ActionBar: *a, OverLog: a.overLog()}
	if v.Filter == "" {
		v.Filter = r.tr("Filter · name, address, MAC")
	}
	if v.Action != nil {
		act := *v.Action
		act.Href = SafeHref(act.Href)
		v.Action = &act
	}
	if a.Drawer != nil {
		panel, err := r.renderPanel(a.Drawer, csrf, Flash{})
		if err != nil {
			return err
		}
		v.HasPanel, v.Open, v.Panel = true, a.Drawer.Open, panel
	}
	return r.execute(out, "actionbar.html.tmpl", v)
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"io"
)

// ActionBar is the row between a page's heading and its listing: which slice of
// the listing you are looking at and how to narrow it. It is the listing's
// controls, not the page's — every part of it acts on the rows below and nothing
// else, which is why it sits with them rather than in the chrome. The page's
// own act is not on it: that is the envelope's (HeadingAct), and stands on the
// heading line.
//
// The parts read left to right in the order a person reaches for them. Tabs are
// the coarse cut, each carrying its own count so the cut is priced before it is
// made. With them, the fine cuts: a free-text Filter and a Select for the one
// dimension a listing is always sliced along.
//
// Everything here narrows what is already on screen, so the whole bar is
// client-side: nothing it does is a request, and nothing it does can fail.
//
// The same type draws the heading line's act (Heading), which is why it holds
// an act's fields; a plugin's bar never sets them, so they are not read from
// its schema.
type ActionBar struct {
	Tabs   []ActionTab `json:"tabs,omitempty"`
	Filter string      `json:"filter,omitempty"` // the search field's placeholder; drawn over a live listing only (searches)
	Select *ActionPick `json:"select,omitempty"`
	// Live is the label of the control that holds a running listing still. It
	// is not a narrowing, so the shell lifts it to the heading line beside the
	// page's act (TakeLive): it reads Live, its spinner turning, while events
	// arrive and Paused while they are held, and pressing it is how that
	// changes. The shell owns the rest of that behaviour (verso-listing.js).
	Live string `json:"live,omitempty"`
	// Action, Entity, OpensPanel and Drawer are the heading line's act and what
	// it opens: an entity's panel (Entity), a panel the plugin renders at the
	// act's own address (OpensPanel), or the blank object's panel it carries
	// (Drawer), whose Open makes an address asking for a new object arrive
	// with the panel in front of the operator. The Action's Href is what a
	// browser with no script follows.
	Action     *TableAction `json:"-"`
	Entity     string       `json:"-"`
	OpensPanel bool         `json:"-"`
	Drawer     *RowDrawer   `json:"-"`
	// Heading renders the act alone, for the heading line: set by the shell,
	// never by a plugin.
	Heading bool `json:"-"`
	// liveLog marks a live log's bar whose live control the shell lifted to
	// the heading line: it still sits over a log, not a table.
	liveLog bool
}

// HeadingAct is a page's one act, which stands on its heading line beside the
// title: making a new subject of the kind the page lists, or, quietly, taking
// something away from it (a log's download). It travels beside the page in the
// envelope rather than inside it, because the heading line is the page's, not
// the listing's. Making one is editing one that does not exist yet, so an act
// may open a panel: OpensPanel says its address is a panel the plugin renders,
// Drawer carries the blank object's panel itself.
type HeadingAct struct {
	Label      string     `json:"label"`
	Href       string     `json:"href"`
	Icon       string     `json:"icon,omitempty"`
	Style      string     `json:"style,omitempty"` // "quiet": the act takes something away rather than making something
	OpensPanel bool       `json:"opens_panel,omitempty"`
	Drawer     *RowDrawer `json:"drawer,omitempty"`
}

// DecodeHeadingAct reads an envelope's act; no act is nil.
func DecodeHeadingAct(data []byte) (*HeadingAct, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	act := new(HeadingAct)
	if err := json.Unmarshal(data, act); err != nil {
		return nil, err
	}
	return act, nil
}

// Bar is the act as the heading line draws it; no act draws nothing.
func (a *HeadingAct) Bar() *ActionBar {
	if a == nil {
		return nil
	}
	return &ActionBar{
		Action:     &TableAction{Label: a.Label, Href: a.Href, Icon: a.Icon, Style: a.Style},
		OpensPanel: a.OpensPanel,
		Drawer:     a.Drawer,
		Heading:    true,
	}
}

// overLog reports whether the bar sits over a live log rather than a table,
// which it wears unfilled.
func (a *ActionBar) overLog() bool { return a.Live != "" || a.liveLog }

// headed is the page with its act riding in front of it, from decoding until
// the heading line is drawn (WithHeading, SplitHeading).
type headed struct {
	Stack
}

// WithHeading puts a page's act in front of its tree, so every pass the shell
// makes over the page — staging marks, refusals, the open panel, the frame a
// panel's forms post into — reaches the act's blank panel as it reaches a
// row's. No act leaves the page as it is.
func WithHeading(act *HeadingAct, w Widget) Widget {
	if act == nil {
		return w
	}
	return &headed{Stack{Children: []Widget{act.Bar(), w}}}
}

// SplitHeading takes the act back off for the heading line, returning it and
// the page as it was; a page that carried none gives none.
func SplitHeading(w Widget) (*ActionBar, Widget) {
	h, ok := w.(*headed)
	if !ok || len(h.Children) != 2 {
		return nil, w
	}
	bar, _ := h.Children[0].(*ActionBar)
	return bar, h.Children[1]
}

// TakeLive lifts a live log's live control onto its heading line, where it
// stands beside the page's act as its equal; the bar keeps what narrows the
// log, in the log's dress. Only a bar leading the page's Stack has a heading to
// give it to; any other shape gives nothing and "" is returned.
func TakeLive(w Widget) string {
	stack, ok := w.(*Stack)
	if !ok || len(stack.Children) == 0 {
		return ""
	}
	bar, ok := stack.Children[0].(*ActionBar)
	if !ok || bar.Live == "" {
		return ""
	}
	live := bar.Live
	bar.liveLog, bar.Live = true, ""
	return live
}

// searches reports whether the bar draws its search field. Only over a live
// listing: a listing that holds what it has is read by scrolling and found in
// with the browser's own find, however long it is, while a live one's rows
// arrive as it is read and the browser's find cannot hold a question across
// them.
func (a *ActionBar) searches() bool { return a.overLog() }

// bare reports whether the bar has nothing left to show: no search it draws,
// no cut, no select, no act and no live control.
func (a *ActionBar) bare() bool {
	return (a.Filter == "" || !a.searches()) && len(a.Tabs) == 0 && a.Select == nil &&
		a.Action == nil && a.Live == "" && !a.Heading
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
	if a.bare() && a.Drawer == nil {
		return nil
	}
	v := actionBarView{ActionBar: *a, OverLog: a.overLog()}
	switch {
	case !a.searches():
		v.Filter = ""
	case v.Filter == "":
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

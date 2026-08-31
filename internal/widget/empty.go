// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Empty is a first-run / nothing-here state: a friendly icon, a headline, a line of
// plain-language reassurance, and the one action that gets started. It turns a blank
// list into an invitation instead of a dead end — the humane answer to "there's
// nothing to show yet." The children are the call(s) to action (typically a modal
// trigger), so the empty state composes the same widgets as the populated one.
type Empty struct {
	Icon     string   `json:"icon"`  // "shield" | "globe" | "device" (default)
	Title    string   `json:"title"` // headline, e.g. "Reach your home from anywhere"
	Body     string   `json:"body"`  // one line of reassurance
	Children []Widget // the call(s) to action
}

func (*Empty) isWidget() {}

func (e *Empty) children() []Widget { return e.Children }

func (e *Empty) prune(keep func(Widget) bool) { e.Children = pruneList(e.Children, keep) }

// UnmarshalJSON decodes the action children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing.
func (e *Empty) UnmarshalJSON(data []byte) error {
	var raw struct {
		Icon     string            `json:"icon"`
		Title    string            `json:"title"`
		Body     string            `json:"body"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Icon, e.Title, e.Body = raw.Icon, raw.Title, raw.Body
	children, err := decodeChildren(raw.Children, "empty child")
	if err != nil {
		return err
	}
	e.Children = children
	return nil
}

// emptyView is the empty template's model: the icon/title/body plus the
// call-to-action children already rendered to trusted HTML.
type emptyView struct {
	Icon, Title, Body string
	Children          []template.HTML
}

// renderInto renders the call-to-action children through the renderer, then hands the
// template the first-run chrome (icon, headline, reassurance). The plugin declared the
// copy and the actions; the presentation is the shell's (ADR-005 §7).
func (e *Empty) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(e.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "empty.html.tmpl", emptyView{Icon: e.Icon, Title: e.Title, Body: e.Body, Children: children})
}

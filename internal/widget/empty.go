// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Empty is a whole-page state that is not a listing: a takeover's phase, a page
// waiting on a precondition — an icon, a headline, a line of plain-language
// reassurance, and the one action that moves it on. A listing with nothing in it
// never uses it; the table says its nothing in one row (Table.EmptyText). The
// children are the call(s) to action, so the state composes the same widgets as
// the page around it.
type Empty struct {
	Icon  string `json:"icon"`  // "shield" | "globe" | "device" (default)
	Title string `json:"title"` // headline, e.g. "Reach your home from anywhere"
	Body  string `json:"body"`  // one line of reassurance (Markdown)
	// Variant tones the icon by the badge vocabulary — "success" for a state
	// that has arrived (an upgrade finished) rather than one waiting to begin.
	// Absent, the icon wears the accent; an unknown value falls back to it.
	Variant  string   `json:"variant,omitempty"`
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
		Variant  string            `json:"variant"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Icon, e.Title, e.Body, e.Variant = raw.Icon, raw.Title, raw.Body, raw.Variant
	children, err := decodeChildren(raw.Children, "empty child")
	if err != nil {
		return err
	}
	e.Children = children
	return nil
}

// emptyView is the empty template's model: the icon/title plus the reassurance
// and the call-to-action children already rendered to trusted HTML.
type emptyView struct {
	Icon, Title, Variant string
	Body                 template.HTML
	Children             []template.HTML
}

// renderInto renders the call-to-action children through the renderer, then hands the
// template the first-run chrome (icon, headline, reassurance). The plugin declared the
// copy and the actions; the presentation is the shell's (ADR-005 §7).
//
// The body takes the same Markdown pass a section's sub does, through the same
// sanitising engine: this is the one line that tells a person what to do next,
// and it routinely has to name the control that does it. Raw HTML is neutralised
// there, so declaring prose can never become declaring markup.
func (e *Empty) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(e.Children, csrf)
	if err != nil {
		return err
	}
	v := emptyView{Icon: e.Icon, Title: e.Title, Variant: e.Variant, Children: children}
	if e.Body != "" {
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(e.Body), &buf); err != nil {
			return fmt.Errorf("widget: render empty body: %w", err)
		}
		v.Body = template.HTML(buf.String())
	}
	return r.execute(out, "empty.html.tmpl", v)
}

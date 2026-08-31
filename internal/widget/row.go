// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// Row is one item in a list: an optional leading icon, a title with optional
// secondary meta text, and an optional trailing status badge. It is the compact
// building block for lists of things-with-status — devices, interfaces, services —
// so they read as scannable rows rather than stacked boxes.
type Row struct {
	Icon    string `json:"icon"` // "phone" | "laptop" | "router" | "device" (default) | "" (none)
	Title   string `json:"title"`
	Meta    string `json:"meta"`    // secondary text, inline after the title
	Tag     string `json:"tag"`     // small category chip after the meta (e.g. a network/zone); omitted when empty
	Columns bool   `json:"columns"` // give title/meta/tag fixed widths so a list of rows aligns into columns (scannable)
	Status  *Badge `json:"status"`  // optional trailing status pill
	// Chevron is the "this opens" cue. It is opt-in and must be honest: set
	// it only when the row actually goes somewhere — as a drawer trigger or a
	// link — never as decoration on a purely informational row.
	Chevron bool `json:"chevron,omitempty"`
}

func (*Row) isWidget() {}

func (w *Row) children() []Widget {
	if w.Status == nil {
		return nil
	}
	return []Widget{w.Status}
}

// rowView is the row template's model: its optional status badge pre-rendered to
// trusted HTML (by this renderer), the rest plain text the template escapes.
type rowView struct {
	Icon, Title, Meta, Tag string
	Columns, Chevron       bool
	Status                 template.HTML
}

func (w *Row) renderInto(r *Renderer, out io.Writer, _ string) error {
	var status template.HTML
	if w.Status != nil {
		var b strings.Builder
		if err := r.execute(&b, "badge.html.tmpl", w.Status); err != nil {
			return err
		}
		status = template.HTML(b.String())
	}
	return r.execute(out, "row.html.tmpl", rowView{Icon: w.Icon, Title: w.Title, Meta: w.Meta, Tag: w.Tag, Columns: w.Columns, Chevron: w.Chevron, Status: status})
}

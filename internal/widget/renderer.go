// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
	"sync/atomic"

	"github.com/yuin/goldmark"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Renderer renders widgets to HTML. Values flow through html/template, so
// plugin-supplied text is contextually escaped and cannot inject markup into
// the shell's origin. Not to be copied after first use (it holds counters).
//
// Dispatch is polymorphic, not a switch: each widget renders itself via renderInto
// (below), so adding a widget touches only that widget's file — never this engine.
// The per-widget view models and render logic live beside their structs.
type Renderer struct {
	tmpl     *template.Template
	md       goldmark.Markdown
	rawUses  atomic.Int64
	tabSeq   atomic.Int64 // per-render unique id, so multiple tabs groups never collide
	wizSeq   atomic.Int64 // ditto for wizards
	cfmSeq   atomic.Int64 // ditto for confirm widgets
	chartSeq atomic.Int64 // ditto for charts' gradient ids
}

// NewRenderer parses the embedded widget templates and builds the sanitising
// Markdown engine for the raw bridge.
func NewRenderer() (*Renderer, error) {
	tmpl, err := template.New("widget").Funcs(template.FuncMap{"icon": Icon}).ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("widget: parse templates: %w", err)
	}
	return &Renderer{tmpl: tmpl, md: newMarkdown()}, nil
}

// Render writes the HTML for w. The widget set is closed, so an unknown type is
// a programming error, not an extension point.
func (r *Renderer) Render(out io.Writer, w Widget) error {
	return r.render(out, w, "")
}

// RenderWithToken renders w, injecting csrfToken as a hidden field into any form
// it produces, so state-changing plugin submissions carry the caller's CSRF
// token (VS-04). Plain Render omits it.
func (r *Renderer) RenderWithToken(out io.Writer, w Widget, csrfToken string) error {
	return r.render(out, w, csrfToken)
}

// render dispatches to the widget itself — polymorphism replaces the old type
// switch (Open/Closed). Containers call it back on their children for recursion.
func (r *Renderer) render(out io.Writer, w Widget, csrf string) error {
	return w.renderInto(r, out, csrf)
}

// renderChildren renders a slice of widgets to trusted HTML fragments (each produced
// by this renderer, so html/template's escaping guarantees still hold) — the shared
// helper every container widget uses to render its subtree.
func (r *Renderer) renderChildren(children []Widget, csrf string) ([]template.HTML, error) {
	out := make([]template.HTML, 0, len(children))
	for _, c := range children {
		var b strings.Builder
		if err := r.render(&b, c, csrf); err != nil {
			return nil, err
		}
		out = append(out, template.HTML(b.String()))
	}
	return out, nil
}

// joinHTML concatenates rendered fragments into one — for containers whose template
// takes a single body blob (a tab panel, a wizard step) rather than ranging a slice.
func joinHTML(parts []template.HTML) template.HTML {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(string(p))
	}
	return template.HTML(b.String())
}

// RawUsage reports how many raw blocks this renderer has rendered — the demand
// signal that tells us which widget to build next (ADR-005 §5).
func (r *Renderer) RawUsage() int64 { return r.rawUses.Load() }

func (r *Renderer) execute(out io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(out, name, data); err != nil {
		return fmt.Errorf("widget: render %s: %w", name, err)
	}
	return nil
}

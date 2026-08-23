// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
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
// the shell's origin. Not to be copied after first use (it holds a counter).
type Renderer struct {
	tmpl    *template.Template
	md      goldmark.Markdown
	rawUses atomic.Int64
}

// NewRenderer parses the embedded widget templates and builds the sanitising
// Markdown engine for the raw bridge.
func NewRenderer() (*Renderer, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("widget: parse templates: %w", err)
	}
	return &Renderer{tmpl: tmpl, md: newMarkdown()}, nil
}

// Render writes the HTML for w. The widget set is closed, so an unknown type is
// a programming error, not an extension point.
func (r *Renderer) Render(out io.Writer, w Widget) error {
	switch v := w.(type) {
	case *Table:
		return r.execute(out, "table.html.tmpl", v)
	case *Card:
		return r.renderCard(out, v)
	case *Form:
		return r.renderForm(out, v)
	case *Field:
		return r.execute(out, "field.html.tmpl", v)
	case *List:
		return r.execute(out, "list.html.tmpl", v)
	case *Raw:
		return r.renderRaw(out, v)
	default:
		return fmt.Errorf("widget: no renderer for %T", w)
	}
}

type rawView struct{ HTML template.HTML }

// renderRaw converts the plugin's Markdown through the sanitising engine and
// wraps it in the raw affordance. It bumps the usage counter first: raw usage is
// the demand signal for the next widget (ADR-005 §5).
func (r *Renderer) renderRaw(out io.Writer, w *Raw) error {
	r.rawUses.Add(1)
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(w.Markdown), &buf); err != nil {
		return fmt.Errorf("widget: render raw: %w", err)
	}
	return r.execute(out, "raw.html.tmpl", rawView{HTML: template.HTML(buf.String())})
}

// RawUsage reports how many raw blocks this renderer has rendered — the demand
// signal that tells us which widget to build next (ADR-005 §5).
func (r *Renderer) RawUsage() int64 { return r.rawUses.Load() }

// formView is the form template's model: its fields pre-rendered to trusted HTML
// (each produced by this renderer), plus the resolved submit label.
type formView struct {
	Submit  string
	Success string
	Fields  []template.HTML
}

// renderForm renders a form by first rendering each field through Render, keeping
// composition in Go and the template a dumb shell (as renderCard does).
func (r *Renderer) renderForm(out io.Writer, f *Form) error {
	fields := make([]template.HTML, 0, len(f.Fields))
	for _, field := range f.Fields {
		var b strings.Builder
		if err := r.Render(&b, field); err != nil {
			return err
		}
		fields = append(fields, template.HTML(b.String()))
	}
	submit := f.Submit
	if submit == "" {
		submit = "Save"
	}
	return r.execute(out, "form.html.tmpl", formView{
		Submit: submit, Success: f.Success, Fields: fields,
	})
}

// cardView is the card template's model: the title plus its children already
// rendered to trusted HTML fragments (each produced by this same renderer, so
// html/template's escaping guarantees still hold).
type cardView struct {
	Title    string
	Children []template.HTML
}

// renderCard renders a card by first rendering each child through Render, so
// composition/nesting lives in Go and the template stays a dumb shell.
func (r *Renderer) renderCard(out io.Writer, c *Card) error {
	children := make([]template.HTML, 0, len(c.Children))
	for _, child := range c.Children {
		var b strings.Builder
		if err := r.Render(&b, child); err != nil {
			return err
		}
		children = append(children, template.HTML(b.String()))
	}
	return r.execute(out, "card.html.tmpl", cardView{Title: c.Title, Children: children})
}

func (r *Renderer) execute(out io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(out, name, data); err != nil {
		return fmt.Errorf("widget: render %s: %w", name, err)
	}
	return nil
}

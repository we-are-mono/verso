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
	return r.render(out, w, "")
}

// RenderWithToken renders w, injecting csrfToken as a hidden field into any form
// it produces, so state-changing plugin submissions carry the caller's CSRF
// token (VS-04). Plain Render omits it.
func (r *Renderer) RenderWithToken(out io.Writer, w Widget, csrfToken string) error {
	return r.render(out, w, csrfToken)
}

func (r *Renderer) render(out io.Writer, w Widget, csrf string) error {
	switch v := w.(type) {
	case *Table:
		return r.execute(out, "table.html.tmpl", v)
	case *Card:
		return r.renderCard(out, v, csrf)
	case *Form:
		return r.renderForm(out, v, csrf)
	case *Field:
		return r.execute(out, "field.html.tmpl", v)
	case *List:
		return r.execute(out, "list.html.tmpl", v)
	case *Raw:
		return r.renderRaw(out, v)
	case *Repeater:
		return r.renderRepeater(out, v, csrf)
	case *Conditional:
		return r.renderConditional(out, v, csrf)
	default:
		return fmt.Errorf("widget: no renderer for %T", w)
	}
}

// conditionalView is the conditional template's model: the toggle plus the gated
// fields, already rendered to trusted HTML.
type conditionalView struct {
	Name, Label string
	Checked     bool
	Fields      []template.HTML
}

// renderConditional renders the gated field-set through render, then hands the
// template the controlling toggle. Visibility is pure CSS (ADR-005 §7): the shell
// owns the toggle and the show/hide, the plugin only declared the intent.
func (r *Renderer) renderConditional(out io.Writer, c *Conditional, csrf string) error {
	fields := make([]template.HTML, 0, len(c.Fields))
	for _, f := range c.Fields {
		var b strings.Builder
		if err := r.render(&b, f, csrf); err != nil {
			return err
		}
		fields = append(fields, template.HTML(b.String()))
	}
	return r.execute(out, "conditional.html.tmpl", conditionalView{
		Name: c.Name, Label: c.Label, Checked: c.Checked, Fields: fields,
	})
}

// repeaterItemView is one rendered item plus the uci section it maps to, so the
// remove affordance can name it.
type repeaterItemView struct {
	Section string
	Body    template.HTML
}

// repeaterView is the repeater template's model: the pre-rendered items, the
// declared uci backing, and the hidden-field names the affordances post (from the
// package constants, so the template and the gateway never drift).
type repeaterView struct {
	Config, SectionType, AddLabel, CSRFToken      string
	Items                                         []repeaterItemView
	OpField, ConfigField, TypeField, SectionField string
	OpAdd, OpRemove                               string
}

// renderRepeater renders each item's subtree through render (threading the CSRF
// token to any form inside it), then hands the template the shell-owned add/remove
// affordances. The plugin supplied only the items and the declaration; every
// affordance and its wiring is the shell's (ADR-005 §7).
func (r *Renderer) renderRepeater(out io.Writer, rp *Repeater, csrf string) error {
	items := make([]repeaterItemView, 0, len(rp.Items))
	for _, it := range rp.Items {
		var b strings.Builder
		if err := r.render(&b, it.Widget, csrf); err != nil {
			return err
		}
		items = append(items, repeaterItemView{Section: it.Section, Body: template.HTML(b.String())})
	}
	add := rp.AddLabel
	if add == "" {
		add = "Add"
	}
	return r.execute(out, "repeater.html.tmpl", repeaterView{
		Config: rp.Config, SectionType: rp.SectionType, AddLabel: add, CSRFToken: csrf,
		Items:        items,
		OpField:      RepeaterOpField,
		ConfigField:  RepeaterConfigField,
		TypeField:    RepeaterTypeField,
		SectionField: RepeaterSectionField,
		OpAdd:        RepeaterOpAdd,
		OpRemove:     RepeaterOpRemove,
	})
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
	Submit    string
	Success   string
	Error     string
	CSRFToken string
	Actions   []FormAction
	Fields    []template.HTML
}

// renderForm renders a form by first rendering each field through render, keeping
// composition in Go and the template a dumb shell (as renderCard does).
func (r *Renderer) renderForm(out io.Writer, f *Form, csrf string) error {
	fields := make([]template.HTML, 0, len(f.Fields))
	for _, field := range f.Fields {
		var b strings.Builder
		if err := r.render(&b, field, csrf); err != nil {
			return err
		}
		fields = append(fields, template.HTML(b.String()))
	}
	submit := f.Submit
	if submit == "" {
		submit = "Save"
	}
	return r.execute(out, "form.html.tmpl", formView{
		Submit: submit, Success: f.Success, Error: f.Error, CSRFToken: csrf,
		Actions: f.Actions, Fields: fields,
	})
}

// cardView is the card template's model: the title plus its children already
// rendered to trusted HTML fragments (each produced by this same renderer, so
// html/template's escaping guarantees still hold).
type cardView struct {
	Title    string
	Children []template.HTML
}

// renderCard renders a card by first rendering each child through render, so
// composition/nesting lives in Go and the template stays a dumb shell. The CSRF
// token flows to any nested form.
func (r *Renderer) renderCard(out io.Writer, c *Card, csrf string) error {
	children := make([]template.HTML, 0, len(c.Children))
	for _, child := range c.Children {
		var b strings.Builder
		if err := r.render(&b, child, csrf); err != nil {
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

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Renderer renders widgets to HTML. Values flow through html/template, so
// plugin-supplied text is contextually escaped and cannot inject markup into
// the shell's origin.
type Renderer struct {
	tmpl *template.Template
}

// NewRenderer parses the embedded widget templates.
func NewRenderer() (*Renderer, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("widget: parse templates: %w", err)
	}
	return &Renderer{tmpl: tmpl}, nil
}

// Render writes the HTML for w. The widget set is closed, so an unknown type is
// a programming error, not an extension point.
func (r *Renderer) Render(out io.Writer, w Widget) error {
	switch v := w.(type) {
	case *Table:
		return r.execute(out, "table.html.tmpl", v)
	default:
		return fmt.Errorf("widget: no renderer for %T", w)
	}
}

func (r *Renderer) execute(out io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(out, name, data); err != nil {
		return fmt.Errorf("widget: render %s: %w", name, err)
	}
	return nil
}

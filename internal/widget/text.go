// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
)

// Text is first-class display prose. Unlike raw — a deliberately-marked escape
// hatch (ADR-005 §5) — text is normal page content: Markdown rendered through the
// shell's styling with no "raw" affordance. Use it for headings, notes, and
// labelled values; reach for raw only when a real widget is genuinely missing.
type Text struct {
	Markdown string `json:"markdown"`
	// Translated marks prose the shell composed from strings it already
	// translated (a count filled into its sentence), which a second pass would
	// look up as a key it never was. Shell only.
	Translated bool `json:"-"`
}

func (*Text) isWidget() {}

func (*Text) children() []Widget { return nil }

// renderInto runs the prose through the same sanitizing Markdown as raw, but with
// no "raw" affordance — it is first-class content, not an escape hatch.
func (t *Text) renderInto(r *Renderer, out io.Writer, _ string) error {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(t.Markdown), &buf); err != nil {
		return fmt.Errorf("widget: render text: %w", err)
	}
	return r.execute(out, "text.html.tmpl", template.HTML(buf.String()))
}

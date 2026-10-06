// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
)

// Embed is a block that arrives already drawn: a plugin's contribution to a
// shell page (SSH on Access), rendered by the plugin pipeline and set into the
// page's own stack, so the stack spaces it as it spaces any of its blocks. It
// is the shell's alone: Decode never makes one, so a plugin cannot send HTML
// through it.
type Embed struct {
	HTML template.HTML `json:"-"`
}

func (*Embed) isWidget() {}

func (*Embed) children() []Widget { return nil }

func (w *Embed) renderInto(_ *Renderer, out io.Writer, _ string) error {
	_, err := io.WriteString(out, string(w.HTML))
	return err
}

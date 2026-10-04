// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Divider is a section separator: a light horizontal rule that sets groups of
// content apart on a long page.
type Divider struct {
	Tight bool `json:"tight"` // compact spacing, for separating groups inside a card/panel
}

func (*Divider) isWidget() {}

func (*Divider) children() []Widget { return nil }

func (d *Divider) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "divider.html.tmpl", d)
}

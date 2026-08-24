// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Divider is a section separator: a light horizontal rule with an optional label
// centred on the line, the rule breaking cleanly around the label. It sets groups
// of content apart on a long page — an <hr> with an optional legend.
type Divider struct {
	Label string `json:"label"`
}

func (*Divider) isWidget() {}

func (d *Divider) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "divider.html.tmpl", d)
}

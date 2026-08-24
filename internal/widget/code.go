// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Code displays a long machine value — a public key, a token, an ID — in a
// full-width monospace box with an optional inline copy. It is the right home for
// the kind of string a properties row would cram: it gets room to breathe and wrap,
// and a one-tap copy sits beside it.
type Code struct {
	Label string `json:"label"` // optional heading above the box
	Value string `json:"value"` // the machine value shown (and copied)
	Copy  bool   `json:"copy"`  // show an inline copy button
}

func (*Code) isWidget() {}

func (c *Code) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "code.html.tmpl", c)
}

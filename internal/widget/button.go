// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Button is a direct action control. Without Name it is intentionally inert,
// useful for a static preview; with Name it submits its Value from a containing
// form. Style selects semantic emphasis rather than arbitrary classes.
type Button struct {
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Style string `json:"style,omitempty"` // "" / "primary" | "secondary" | "danger"
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

func (*Button) isWidget() {}

func (b *Button) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "button.html.tmpl", b)
}

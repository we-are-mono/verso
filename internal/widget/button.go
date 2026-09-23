// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Button is a direct action control. Without Name it is intentionally inert,
// useful for a static preview; with Name it submits its Value from a containing
// form. Style selects semantic emphasis rather than arbitrary classes.
type Button struct {
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Style    string `json:"style,omitempty"` // "" / "primary" | "secondary" | "ghost" | "danger" | "act" (an act on a part of a section, in the subsection act's dress)
	Name     string `json:"name,omitempty"`
	Value    string `json:"value,omitempty"`
	Disabled bool   `json:"disabled,omitempty"` // unavailable action; rendered natively disabled
	Loading  bool   `json:"loading,omitempty"`  // disabled busy state; replaces Icon with a spinning loader
	// Live states that something the button governs is running — a stream
	// flowing, a sampler sampling — and the spinner says so. Unlike Loading it
	// takes nothing away: the button stays enabled, keeps its hover and its
	// pointer, and is meant to be pressed (that is how the running thing
	// stops). Loading is the in-flight submit; Live is the live indicator.
	Live bool `json:"live,omitempty"`
}

func (*Button) isWidget() {}

func (*Button) children() []Widget { return nil }

func (b *Button) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "button.html.tmpl", b)
}

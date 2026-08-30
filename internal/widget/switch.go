// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Switch is one compact persistent on/off setting. It uses the same control as
// table toggle cells, so an object's enabled state has one visual language in
// summaries and editors.
type Switch struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	OffLabel string `json:"off_label,omitempty"`
	Help     string `json:"help,omitempty"`
	Style    string `json:"style,omitempty"` // "" labelled row | "inline" beside a section heading
	On       bool   `json:"on,omitempty"`
}

func (*Switch) isWidget() {}

func (s *Switch) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "form_switch.html.tmpl", s)
}

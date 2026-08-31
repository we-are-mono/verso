// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Badge is a small status pill: a semantic variant (never a colour, ADR-005) plus
// short text, optionally with a leading status dot. The shell maps the variant to
// its palette, so "connected" always looks the same across the UI.
type Badge struct {
	Variant string `json:"variant"` // "neutral" (default) | "success" | "warning" | "danger" | "info"
	Text    string `json:"text"`
	Dot     bool   `json:"dot"`            // show a leading status dot (e.g. online/offline)
	Icon    string `json:"icon,omitempty"` // optional leading icon, by Lucide name (instead of, or beside, the dot)
	Size    string `json:"size,omitempty"` // "" (pill, the in-grid scale) | "lg" — a standalone banner chip
}

func (*Badge) isWidget() {}

func (*Badge) children() []Widget { return nil }

func (b *Badge) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "badge.html.tmpl", b)
}

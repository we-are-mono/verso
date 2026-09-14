// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Badge is a small status pill: a semantic variant (never a colour, ADR-005) plus
// short text, optionally with a leading status dot. The shell maps the variant to
// its palette, so "connected" always looks the same across the UI.
type Badge struct {
	// Variant is the tone vocabulary — the one spelling every widget's semantic
	// state speaks (a badge/callout variant, a stat/hero verdict, a meter band,
	// a status dot): "neutral" (default) | "success" | "warning" | "danger" | "info".
	Variant string `json:"variant"`
	Text    string `json:"text"`
	Dot     bool   `json:"dot"`            // show a leading status dot (e.g. online/offline)
	Icon    string `json:"icon,omitempty"` // optional leading icon, by Lucide name (instead of, or beside, the dot)
	Size    string `json:"size,omitempty"` // "" (pill, the in-grid scale) | "lg" — a standalone banner chip
	// Plain drops the pill entirely — no fill, no ring, no chip padding — leaving a
	// leading tone dot (or icon) and plain-ink text. It is the resting status line,
	// where the dot carries the state and the words stay words (the stage's own
	// "unsaved changes" note), not a chip to be read as a chip.
	Plain bool `json:"plain,omitempty"`
}

func (*Badge) isWidget() {}

func (*Badge) children() []Widget { return nil }

func (b *Badge) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "badge.html.tmpl", b)
}

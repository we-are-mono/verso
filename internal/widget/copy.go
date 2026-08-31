// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Copy is a button that copies a piece of text to the clipboard and briefly
// confirms it — for keys, tunnel addresses, config blobs a person needs to paste
// elsewhere. It is shell-owned behaviour (ADR-005 §7) realized with Alpine
// (ADR-004); the plugin declares the label and the text, the shell owns the copy
// and the "Copied!" feedback (with an execCommand fallback for non-secure origins).
type Copy struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

func (*Copy) isWidget() {}

func (*Copy) children() []Widget { return nil }

func (c *Copy) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "copy.html.tmpl", c)
}

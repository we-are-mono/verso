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
	// Live declares the block a preview of the form it sits in: what the form
	// would write, kept current as the form is edited rather than as it was when
	// the page was built. It is intent and not a mechanism (ADR-005 §7) — the
	// plugin says "this previews the form", and the shell realizes it by asking
	// the plugin to re-render from the values on screen. Only the plugin can
	// answer: which option a switch writes, whether an off is a zero or a
	// deletion, and how a mark becomes `!0x10/0xff` are its readings of the
	// daemon's grammar, and a preview the shell computed for itself would be a
	// second copy of them, drifting.
	Live bool `json:"live,omitempty"`
}

func (*Code) isWidget() {}

func (*Code) children() []Widget { return nil }

func (c *Code) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "code.html.tmpl", c)
}

// RenderLivePreviewWithToken renders just the live preview a tree carries, for
// a request that is asking only what the form would write. The page around it is
// already on screen and is not being replaced — re-rendering it would take the
// operator's cursor out of the field they are typing in — so this takes the one
// block out of the answer and leaves the rest unsent.
//
// It reports false for a tree that declares no live preview, which is a POST
// that arrived marked as one and should be answered as whatever it really is.
func (r *Renderer) RenderLivePreviewWithToken(out io.Writer, w Widget, csrfToken, lang string, t func(string) string) (bool, error) {
	if t != nil {
		translateSchema(w, t)
	}
	preview := livePreview(w)
	if preview == nil {
		return false, nil
	}
	pass := &Renderer{tmpl: r.setFor(lang), t: t, md: r.md, seq: r.seq}
	return true, pass.render(out, preview, csrfToken)
}

// livePreview is the one block a tree declares as its form's preview. A page
// composing two is a page whose forms would overwrite each other's previews, so
// the first is the one that answers and the gauge below names the rest.
func livePreview(w Widget) *Code {
	var found *Code
	Walk(w, func(n Widget) {
		if code, ok := n.(*Code); ok && code.Live && found == nil {
			found = code
		}
	})
	return found
}

// LivePreviewCount is how many live previews a tree declares — the dev gauge's
// question, since only the first of them can ever be kept current.
func LivePreviewCount(w Widget) int {
	count := 0
	Walk(w, func(n Widget) {
		if code, ok := n.(*Code); ok && code.Live {
			count++
		}
	})
	return count
}

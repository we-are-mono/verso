// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Progress is an indeterminate, in-context waiting state. It tells the person
// what the system is doing without inventing a percentage it cannot know.
type Progress struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

func (*Progress) isWidget() {}

func (p *Progress) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "progress.html.tmpl", p)
}

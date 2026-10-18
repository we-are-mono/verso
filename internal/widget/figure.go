// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
)

// Figure is a picture standing beside what it shows — a board's photograph
// beside the facts of the box — or, where there is none, a dashed box
// saying why, with the link to how to give it one. The shell composes it; it
// is not part of the plugin schema.
type Figure struct {
	Src           string // the picture's address
	Alt           string // what it shows, for a screen reader; a name, left as it is
	Width, Height int    // its own size in pixels, so the page keeps its place while it loads
	// Note is why there is no picture, said in the square in its place; a
	// figure with a Src has none.
	Note string
	// Link stands under the Note: where to learn how to give it a picture.
	Link *Link
}

func (*Figure) isWidget() {}

func (f *Figure) children() []Widget {
	if f.Link != nil {
		return []Widget{f.Link}
	}
	return nil
}

// Ratio is the picture's height over its width as the style the figure
// carries, so the block can set the picture's height on the 8px grid at any
// width the figure stands.
func (f *Figure) Ratio() template.CSS {
	ratio := 1.0
	if f.Width > 0 && f.Height > 0 {
		ratio = float64(f.Height) / float64(f.Width)
	}
	return template.CSS(fmt.Sprintf("--verso-figure-ratio: %.4f", ratio)) //nolint:gosec // built here from integers, never from input
}

func (f *Figure) renderInto(r *Renderer, out io.Writer, _ string) error {
	var link template.HTML
	if f.Link != nil {
		var buf bytes.Buffer
		if err := f.Link.renderInto(r, &buf, ""); err != nil {
			return err
		}
		link = template.HTML(buf.String()) //nolint:gosec // rendered by the shell's own templates
	}
	return r.execute(out, "figure.html.tmpl", struct {
		*Figure
		RenderedLink template.HTML
	}{Figure: f, RenderedLink: link})
}

// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Canvas is a light inset surface: a rounded panel (slate-50 fill on a slate-200
// hairline) that holds a visual — the network map, a diagram, a chart — set apart
// from the white page without the weight of a card. Children decode recursively
// through Decode, so the closed set is enforced at every level.
//
// Pad sets the padding on every side per canvas, in Tailwind spacing units (as
// in p-N, one unit = 0.25rem); 0 means the default of 12 (3rem). It renders as an
// inline rem value because the amount is per-instance — Tailwind can't compile an
// open-ended set of p-N classes.
type Canvas struct {
	Pad      int
	Children []Widget
}

const canvasDefaultPad = 12

func (*Canvas) isWidget() {}

// UnmarshalJSON decodes the canvas's padding and children, recursing through
// Decode so an unknown child type fails here rather than silently vanishing.
func (c *Canvas) UnmarshalJSON(data []byte) error {
	var raw struct {
		Pad      int               `json:"pad"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Pad = raw.Pad
	c.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		child, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("canvas child %d: %w", i, err)
		}
		c.Children = append(c.Children, child)
	}
	return nil
}

type canvasView struct {
	Pad      string // padding on every side as a rem value, e.g. "3rem"
	Children []template.HTML
}

func (c *Canvas) renderInto(r *Renderer, out io.Writer, csrf string) error {
	kids, err := r.renderChildren(c.Children, csrf)
	if err != nil {
		return err
	}
	units := c.Pad
	if units <= 0 {
		units = canvasDefaultPad
	}
	return r.execute(out, "canvas.html.tmpl", canvasView{
		Pad:      fmt.Sprintf("%grem", float64(units)*0.25),
		Children: kids,
	})
}

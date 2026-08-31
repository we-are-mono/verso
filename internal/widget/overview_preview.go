// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// OverviewPreview is the styleguide workbench for the home page's top block —
// the verdict sentence with its Basic|Advanced switch, the status-tile strip,
// and the IPv4/IPv6 connection facts — rendered from authored data through the
// very template the real page uses (overview.masthead), so visual iteration in
// the styleguide cannot drift from the home page (the capsule-preview pattern).
// It is deliberately inert: no live keys, no streams — data in, block out.
type OverviewPreview struct {
	Kicker string                   `json:"kicker"` // eyebrow above the headline, e.g. "ALL GOOD"
	Lead   string                   `json:"lead"`   // the headline up to the accent word
	Accent string                   `json:"accent"` // the emerald accent word, e.g. "healthy"
	Tiles  []OverviewPreviewTile    `json:"tiles"`  // 3 tiles, or 4 with Wi-Fi
	Facts  []OverviewPreviewFactCol `json:"facts"`  // the two connection-facts columns
}

// OverviewPreviewTile is one status tile of the strip.
type OverviewPreviewTile struct {
	Label   string `json:"label"`
	Icon    string `json:"icon"`
	Variant string `json:"variant"` // "success" | "warning"
	Status  string `json:"status"`
	Caption string `json:"caption"`
}

// OverviewPreviewFactCol is one connection-facts column: an eyebrow (kind +
// protocol) over its label/value rows. The first column sits left of the
// divider, the second right.
type OverviewPreviewFactCol struct {
	Kind  string                `json:"kind"`
	Proto string                `json:"proto"`
	Facts []OverviewPreviewFact `json:"facts"`
}

// OverviewPreviewFact is one label/value row of a facts column.
type OverviewPreviewFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Copy  bool   `json:"copy"`
}

func (*OverviewPreview) isWidget() {}

func (*OverviewPreview) children() []Widget { return nil }

func (p *OverviewPreview) renderInto(r *Renderer, out io.Writer, csrf string) error {
	tiles := make([]ohTile, 0, len(p.Tiles))
	for _, t := range p.Tiles {
		tiles = append(tiles, ohTile{Label: t.Label, Icon: t.Icon, Variant: t.Variant, Status: t.Status, Caption: t.Caption})
	}
	cols := make([]ohFactColView, 0, len(p.Facts))
	for i, col := range p.Facts {
		items := make([]Property, 0, len(col.Facts))
		for _, f := range col.Facts {
			items = append(items, Property{Label: f.Label, Value: f.Value, Mono: true, Emphasis: true, Copy: f.Copy})
		}
		rows, err := renderToHTML(r, &Properties{Align: "left", Items: items}, csrf)
		if err != nil {
			return err
		}
		side := "left"
		if i > 0 {
			side = "right"
		}
		cols = append(cols, ohFactColView{Kind: col.Kind, Proto: col.Proto, Side: side, Rows: rows})
	}
	return r.execute(out, "overview.masthead", overviewMastheadView{
		Kicker: p.Kicker, Lead: p.Lead, Accent: p.Accent, Tiles: tiles, Facts: cols,
	})
}

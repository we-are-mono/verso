// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html"
	"strings"
)

// The generated panel's measures, in pixels at its own size. Every box edge
// stands on an 8px line and every detail on a 4px one; 1px strokes sit half a
// pixel in. The connectors are the Mono Gateway DK's (profiles/
// mono_gateway-dk/back.svg), so a board without art draws the same family.
const (
	panelHeight   = 136
	panelInset    = 24 // chassis edge to the first and last port
	portWidth     = 48
	portGap       = 8  // between ports of one kind
	portKindGap   = 16 // where the connector kind changes
	roleBaseline  = 16 // the WAN tag, on the 0–24 line over the ports
	jackTop       = 24 // an RJ45 body's top; an SFP+ cage's is 8 lower
	cageDrop      = 8
	bodyBottom    = 64
	lipBottom     = 70
	labelBaseline = 96  // the name, on the 80–104 line
	speedBaseline = 120 // the speed, on the 104–128 line
)

// panelText is the generated panel's words as a board's art sets its own: in
// the drawing, at the drawing's size, in the shell's mono face (the head of
// the stylesheet's --font-mono stack), so the panel scales as one picture.
const panelText = ` font-family="Inconsolata, Menlo, monospace" font-size="11"`

// generatedPanel draws a whole panel for a board without art: a chassis
// holding its ports left to right, each with its name and speed under it. It
// carries the same data-verso-port groups, LEDs and outline as board art, so
// litPanel, the stylesheet and the port detail treat it exactly as they treat
// a board's own. It paints nothing itself: every part is an element of the
// panel block (blocks/panel.css), coloured in light and dark.
func generatedPanel(items []PortItem) string {
	x := panelInset
	var body strings.Builder
	for i, it := range items {
		if i > 0 {
			x += portGap
			if it.Kind != items[i-1].Kind {
				x += portKindGap - portGap
			}
		}
		writePort(&body, x, it)
		x += portWidth
	}
	width := x + panelInset
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" class="verso-panel__generated" viewBox="0 0 %d %d" width="%d" height="%d">`, width, panelHeight, width, panelHeight)
	fmt.Fprintf(&b, `<rect class="verso-panel__chassis" x="0.5" y="0.5" width="%d" height="%d" rx="8"/>`, width-1, panelHeight-1)
	b.WriteString(body.String())
	b.WriteString(`</svg>`)
	return b.String()
}

// writePort draws one connector with its box's left edge at x.
func writePort(b *strings.Builder, x int, it PortItem) {
	top := jackTop
	if it.Kind == "sfp" {
		top += cageDrop
	}
	if it.Iface != "" {
		fmt.Fprintf(b, `<g data-verso-port="%s" class="port">`, html.EscapeString(it.Iface))
	} else {
		b.WriteString(`<g class="port">`)
	}
	f := func(v float64) string { return fmt.Sprintf("%g", float64(x)+v) }
	// The outline with its lip: the group's only direct-child path, which the
	// WAN mark strokes.
	fmt.Fprintf(b, `<path class="verso-panel__body" d="M%s,%g H%s A3,3 0 0 1 %s,%g V%g A3,3 0 0 1 %s,%g H%s V%g A1.5,1.5 0 0 1 %s,%g H%s A1.5,1.5 0 0 1 %s,%g V%g H%s A3,3 0 0 1 %s,%g V%g A3,3 0 0 1 %s,%g Z"/>`,
		f(3.5), float64(top)+0.5, f(44.5), f(47.5), float64(top)+3.5, bodyBottom-3.5, f(44.5), bodyBottom-0.5,
		f(35.5), lipBottom-2.0, f(34), lipBottom-0.5, f(14), f(12.5), lipBottom-2.0, bodyBottom-0.5,
		f(3.5), f(0.5), bodyBottom-3.5, float64(top)+3.5, f(3.5), float64(top)+0.5)
	if it.Kind == "sfp" {
		fmt.Fprintf(b, `<rect class="verso-panel__mouth" x="%s" y="%d" width="40" height="%d" rx="1.5"/>`, f(4), top+4, bodyBottom-top-8)
		fmt.Fprintf(b, `<rect class="verso-panel__socket" x="%s" y="%d" width="24" height="8" rx="1"/>`, f(12), bodyBottom-18)
		fmt.Fprintf(b, `<rect class="verso-panel__slit" x="%s" y="%g" width="18" height="1.5" rx="0.5"/>`, f(15), bodyBottom-14.75)
	} else {
		// The mouth follows the outline and its lip 4 in all round; it sits in
		// a group so the outline stays the only direct-child path.
		fmt.Fprintf(b, `<g class="verso-panel__mouth"><path d="M%s,%d H%s A1.5,1.5 0 0 1 %s,%g V%g A1.5,1.5 0 0 1 %s,%d H%s V%d A1,1 0 0 1 %s,%d H%s A1,1 0 0 1 %s,%d V%d H%s A1.5,1.5 0 0 1 %s,%g V%g A1.5,1.5 0 0 1 %s,%d Z"/></g>`,
			f(5.5), top+4, f(42.5), f(44), float64(top)+5.5, bodyBottom-5.5, f(42.5), bodyBottom-4,
			f(32), lipBottom-5, f(31), lipBottom-4, f(17), f(16), lipBottom-5, bodyBottom-4,
			f(5.5), f(4), bodyBottom-5.5, float64(top)+5.5, f(5.5), top+4)
		b.WriteString(`<g class="verso-panel__pins">`)
		for pin := range 8 {
			fmt.Fprintf(b, `<rect x="%s" y="%d" width="1.5" height="8" rx="0.5"/>`, f(12.75+3*float64(pin)), top+4)
		}
		b.WriteString(`</g>`)
	}
	// The role slot over the port: empty here, filled by litPanel on the WAN
	// port, so which port is WAN never lives in the drawing.
	fmt.Fprintf(b, `<text class="verso-port-role" x="%s" y="%d" text-anchor="middle"%s></text>`, f(24), roleBaseline, panelText)
	fmt.Fprintf(b, `<rect class="led-link" x="%s" y="%d" width="8" height="4" rx="1"/>`, f(2), bodyBottom+2)
	fmt.Fprintf(b, `<rect class="led-traffic" x="%s" y="%d" width="8" height="4" rx="1"/>`, f(38), bodyBottom+2)
	fmt.Fprintf(b, `<text class="verso-panel__label" x="%s" y="%d" text-anchor="middle"%s>%s</text>`, f(24), labelBaseline, panelText, html.EscapeString(it.Label))
	if it.Iface != "" {
		speed := it.Speed
		if speed == "" {
			speed = "—"
		}
		fmt.Fprintf(b, `<text class="verso-panel__speed" data-verso-port-speed="" x="%s" y="%d" text-anchor="middle"%s>%s</text>`, f(24), speedBaseline, panelText, html.EscapeString(speed))
	}
	b.WriteString(`</g>`)
}

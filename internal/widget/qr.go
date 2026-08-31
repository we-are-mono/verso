// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/skip2/go-qrcode"
)

// Qr renders a real, scannable QR code for a piece of data — a device's tunnel
// config, a join link, a pairing token — so a phone can set itself up by pointing
// its camera, with no keys to copy. It optionally offers the same payload as a
// download for a machine that can't scan (a laptop, another router), presented as a
// quiet link beneath the code so the two read as one unit. The shell generates the
// code server-side as inline SVG (crisp, no external request, CSP-safe).
type Qr struct {
	Data          string `json:"data"`           // the payload encoded into the code
	Caption       string `json:"caption"`        // short instruction shown under the code
	DownloadHref  string `json:"download_href"`  // optional: offer the payload as a download
	DownloadName  string `json:"download_name"`  // download filename
	DownloadLabel string `json:"download_label"` // download link label (default "Download config file")
}

func (*Qr) isWidget() {}

// qrView is the qr template's model: the code as inline SVG (built here, so it is
// trusted), the caption, and an optional download (its href already validated to a
// trusted template.URL by the link policy).
type qrView struct {
	SVG           template.HTML
	Caption       string
	DownloadHref  template.URL
	DownloadName  string
	DownloadLabel string
}

// renderInto encodes the payload as a real QR code and emits it as inline SVG — one
// path of unit squares, crisp at any size, no external request (CSP-safe). The code
// is dark-on-light regardless of theme, so scanners always see the contrast they need.
func (q *Qr) renderInto(r *Renderer, out io.Writer, _ string) error {
	code, err := qrcode.New(q.Data, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("widget: encode qr: %w", err)
	}
	bitmap := code.Bitmap()
	n := len(bitmap)
	var path strings.Builder
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if bitmap[y][x] {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" width="100%%" height="100%%" shape-rendering="crispEdges" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="QR code"><path d="%s" fill="#0b1220"/></svg>`,
		n, n, path.String())

	view := qrView{SVG: template.HTML(svg), Caption: q.Caption, DownloadName: q.DownloadName, DownloadLabel: q.DownloadLabel}
	if q.DownloadHref != "" {
		view.DownloadHref = safeHref(q.DownloadHref, true)
		if view.DownloadLabel == "" {
			view.DownloadLabel = r.tr("Download config file")
		}
	}
	return r.execute(out, "qr.html.tmpl", view)
}

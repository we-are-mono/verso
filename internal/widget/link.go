// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// Link is a labelled hyperlink, optionally styled as a button and optionally a
// download. It is the escape hatch for "take me there" or "save this file" — a
// device's config for a machine that can't scan a QR, a link to docs. The href is
// the plugin's to choose (a shell route, a data: URL); the shell owns the look and
// the URL policy.
type Link struct {
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Href     string `json:"href"`
	Download string `json:"download"` // non-empty => a download with this filename
	Style    string `json:"style"`    // "" (link) | "button" | "ghost" | "secondary"
	NewTab   bool   `json:"new_tab,omitempty"`
}

func (*Link) isWidget() {}

func (*Link) children() []Widget { return nil }

// linkView is the template's model: the href already validated to a trusted
// template.URL by the shell's own policy (below), so it isn't re-neutralised.
type linkView struct {
	Label    string
	Icon     string
	Href     template.URL
	Download string
	Style    string
	NewTab   bool
}

func (l *Link) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "link.html.tmpl", linkView{
		Label: l.Label, Icon: l.Icon, Href: safeHref(l.Href, l.Download != ""), Download: l.Download, Style: l.Style, NewTab: l.NewTab,
	})
}

// safeHref applies the link widget's URL policy, since a download's href is a data:
// URL that html/template would otherwise strip. Relative, http(s), and mailto are
// always allowed; data: is allowed only for downloads and only for plain-text or
// octet-stream payloads (a config file) — never data:text/html or script schemes.
// Anything else collapses to "#".
func safeHref(raw string, download bool) template.URL {
	s := strings.TrimSpace(raw)
	low := strings.ToLower(s)
	colon := strings.IndexByte(low, ':')
	slash := strings.IndexByte(low, '/')
	scheme := ""
	if colon >= 0 && (slash == -1 || colon < slash) {
		scheme = low[:colon]
	}
	switch {
	case scheme == "": // relative
		return template.URL(s)
	case scheme == "http", scheme == "https", scheme == "mailto":
		return template.URL(s)
	case download && strings.HasPrefix(low, "data:text/plain"):
		return template.URL(s)
	case download && strings.HasPrefix(low, "data:application/octet-stream"):
		return template.URL(s)
	default:
		return template.URL("#")
	}
}

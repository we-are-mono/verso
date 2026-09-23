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
// device's config file, a link to docs. The href is
// the plugin's to choose (a shell route, a data: URL); the shell owns the look and
// the URL policy.
type Link struct {
	Desc     string `json:"desc,omitempty"`
	Code     string `json:"code,omitempty"`
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Href     string `json:"href"`
	Download string `json:"download"` // non-empty => a download with this filename
	// Style is "" (an ordinary link), "button", "ghost", "secondary", "act" (an
	// act on a part of a section, in the 28px quiet dress a set's add slot
	// wears), or "rail" — one place on this page in a list of them, beside the
	// work it points into.
	Style  string `json:"style"`
	NewTab bool   `json:"new_tab,omitempty"`
	// Act is the status style's act: the words on the control that changes
	// the status (the Label), where a setting's control stands.
	Act string `json:"act,omitempty"`
	// Panel opens the destination as a drawer over this page rather than
	// leaving it, for a destination that is its own thing (a package to
	// install, a certificate to replace) — the page's address stays the page's.
	// The status and act styles honour it.
	Panel bool `json:"panel,omitempty"`
}

func (*Link) isWidget() {}

func (*Link) children() []Widget { return nil }

// linkView is the template's model: the href already validated to a trusted
// template.URL by the shell's own policy (below), so it isn't re-neutralised.
type linkView struct {
	Desc, Code string
	Label      string
	Icon       string
	Href       template.URL
	Download   string
	Style      string
	NewTab     bool
	Act        string
	Panel      bool
}

func (l *Link) renderInto(r *Renderer, out io.Writer, _ string) error {
	href := l.Href
	// A link to a place on a page — this one, or one it opens (an expanded
	// row's act into the editor's DHCP section) — is a link to a section,
	// whose id the shell namespaces (SectionID); the plugin names it by its
	// anchor.
	if at := strings.IndexByte(href, '#'); at != -1 && at < len(href)-1 {
		href = href[:at+1] + SectionID(href[at+1:])
	}
	return r.execute(out, "link.html.tmpl", linkView{
		Desc: l.Desc, Code: l.Code, Label: l.Label, Icon: l.Icon, Href: safeHref(href, l.Download != ""), Download: l.Download, Style: l.Style, NewTab: l.NewTab,
		Act: l.Act, Panel: l.Panel,
	})
}

// SafeHref applies the link widget's URL policy to a plugin-supplied href that
// lands in shell chrome (a page action, a table's link cell): relative routes,
// http(s), and mailto pass; everything else collapses to a clean "#" rather
// than html/template's visible ZgotmplZ junk.
func SafeHref(raw string) string {
	return string(safeHref(raw, false))
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

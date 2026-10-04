// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"slices"
	"sort"
	"strings"
)

// collapsedSection is a section the rail folds into one row (ADR-009 §2): the
// row is named for the section and opens into its pages, which are every live
// registration filed under it — in the designed order, the rest by label — and
// after them the pages the shell itself owns there. A stopped plugin withdraws
// its page through the same live registration filter every row uses.
type collapsedSection struct {
	Section string
	Root    string   // the section's own address, which leads to its first page
	Order   []string // page labels the design places first, in its order
	Shell   []pageTab
}

var collapsedSections = []collapsedSection{
	{Section: "Network", Root: "/network", Order: []string{"Interfaces", "DHCP", "DNS"}},
	{Section: "System", Root: "/system", Order: []string{"General"}, Shell: []pageTab{
		{Label: "Hardware", Href: "/system/hardware"},
		{Label: "Access", Href: "/system/access"},
		{Label: "Packages", Href: "/system/packages"},
		{Label: "Services", Href: "/system/services"},
		{Label: "Maintenance", Href: "/system/maintenance"},
		{Label: "Logs", Href: "/system/logs"},
	}},
}

// collapsed returns the collapsed section of one name, if it is one.
func collapsed(section string) (collapsedSection, bool) {
	i := slices.IndexFunc(collapsedSections, func(c collapsedSection) bool { return c.Section == section })
	if i < 0 {
		return collapsedSection{}, false
	}
	return collapsedSections[i], true
}

// sectionPages is a collapsed section's pages, the one active path sits on
// marked. A section that is not collapsed has none.
func (s *Server) sectionPages(section, active string) []pageTab {
	c, ok := collapsed(section)
	if !ok {
		return nil
	}
	var tabs []pageTab
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			if entry.Section == section {
				tabs = append(tabs, pageTab{Label: entry.Label, Href: pluginHref(m.ID, entry.Path), PluginID: m.ID})
			}
		}
	}
	rank := func(label string) int {
		if i := slices.Index(c.Order, label); i >= 0 {
			return i
		}
		return len(c.Order)
	}
	sort.SliceStable(tabs, func(a, b int) bool {
		ra, rb := rank(tabs[a].Label), rank(tabs[b].Label)
		if ra != rb {
			return ra < rb
		}
		return ra == len(c.Order) && tabs[a].Label < tabs[b].Label
	})
	tabs = append(tabs, c.Shell...)
	hrefs := make([]string, len(tabs))
	for i, tab := range tabs {
		hrefs[i] = tab.Href
	}
	if i := bestHref(active, hrefs); i >= 0 {
		tabs[i].Active = true
	}
	return tabs
}

// sectionAt is the section a path belongs to: a collapsed section's own root
// and the shell pages under it, or else the section of the most specific
// registration that holds the path. Liveness is not required: a direct URL to a
// stopped plugin still belongs to its section while it explains that the plugin
// is unavailable.
func (s *Server) sectionAt(active string) string {
	for _, c := range collapsedSections {
		if active == c.Root || strings.HasPrefix(active, c.Root+"/") {
			return c.Section
		}
	}
	var sections, hrefs []string
	for _, m := range s.manifestList() {
		for _, entry := range m.Nav {
			sections = append(sections, entry.Section)
			hrefs = append(hrefs, pluginHref(m.ID, entry.Path))
		}
	}
	if i := bestHref(active, hrefs); i >= 0 {
		return sections[i]
	}
	return ""
}

// handleSectionRoot leads a collapsed section's own address to its first page.
// A section with nothing live has nowhere to lead.
func (s *Server) handleSectionRoot(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pages := s.sectionPages(section, "")
		if len(pages) == 0 {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, pages[0].Href, http.StatusSeeOther)
	}
}

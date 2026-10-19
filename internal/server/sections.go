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
// registration filed under it and the pages the shell itself owns there, each
// where it declares its place (placeLess). A stopped plugin withdraws its page
// through the same live registration filter every row uses.
type collapsedSection struct {
	Section string
	Root    string // the section's own address, which leads to its first page
	Place   int    // the section's row's place in the rail
	Shell   []shellPage
}

// shellPage is a page the shell owns in a collapsed section, at a place
// plugin authors are told, so a plugin's page can stand among them.
type shellPage struct {
	Label, Href string
	Place       int
}

var collapsedSections = []collapsedSection{
	{Section: "Network", Root: "/network", Place: networkPlace},
	{Section: "System", Root: "/system", Place: systemPlace, Shell: []shellPage{
		{Label: "Hardware", Href: "/system/hardware", Place: 20},
		{Label: "Access", Href: "/system/access", Place: 30},
		{Label: "Packages", Href: "/system/packages", Place: 40},
		{Label: "Services", Href: "/system/services", Place: 50},
		{Label: "Maintenance", Href: "/system/maintenance", Place: 60},
		{Label: "Logs", Href: "/system/logs", Place: 70},
		{Label: "Events", Href: eventsPath, Place: 80},
		{Label: "Changes", Href: changesPath, Place: 90},
		{Label: "Diagnostics", Href: "/system/diagnostics", Place: 100},
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
	type placedTab struct {
		place place
		tab   pageTab
	}
	var placed []placedTab
	for _, m := range s.manifestList() {
		if !s.probe(m.Endpoint()) {
			continue
		}
		for _, entry := range m.Nav {
			if entry.Section == section {
				placed = append(placed, placedTab{
					place{order: entry.Order, section: section, label: entry.Label, pluginID: m.ID},
					pageTab{Label: entry.Label, Href: pluginHref(m.ID, entry.Path), PluginID: m.ID},
				})
			}
		}
	}
	for _, page := range c.Shell {
		placed = append(placed, placedTab{place{order: fixed(page.Place), section: section, label: page.Label}, pageTab{Label: page.Label, Href: page.Href}})
	}
	sort.SliceStable(placed, func(a, b int) bool { return placeLess(placed[a].place, placed[b].place) })
	tabs := make([]pageTab, len(placed))
	for i, p := range placed {
		tabs[i] = p.tab
	}
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
			s.notFound(w, r)
			return
		}
		http.Redirect(w, r, pages[0].Href, http.StatusSeeOther)
	}
}

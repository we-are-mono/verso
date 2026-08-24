// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"sort"
	"strings"
)

// coreSectionOrder is the shell-owned core taxonomy (ADR-009 §2): these sections
// render first, in exactly this order, ahead of any plugin-introduced section.
// Status is served by the shell itself; Network, Firewall and System are backed
// by bundled first-party plugins. Membership here is a nav-ordering guarantee, not
// a mechanism — every configuring section is still an ADR-006 plugin.
var coreSectionOrder = []string{"Status", "Network", "Firewall", "System"}

// coreRank returns a section title's position in the core taxonomy, and whether
// it is a core section at all. Non-core (plugin-introduced) sections sort after
// all core sections, by title (ADR-009 §5).
func coreRank(title string) (int, bool) {
	for i, t := range coreSectionOrder {
		if t == title {
			return i, true
		}
	}
	return 0, false
}

// navSection is one collapsible group in the sidebar; navLink is one entry. The
// nav is built from discovered plugin manifests plus the built-in Status group,
// so a plugin appears in the chrome without any shell change (ADR-006 §2).
type navSection struct {
	Title string
	Links []navLink
	Open  bool
}

type navLink struct {
	Label  string
	Href   string
	Active bool
}

// buildNav assembles the sidebar for the current path. The shell-owned Status
// group carries the built-in Overview link; plugins are grouped under their
// manifest's nav.section. Sections are then ordered by the core taxonomy
// (ADR-009 §2, §5): core sections first in canonical order, plugin-introduced
// sections after by title — so the sidebar is deterministic regardless of plugin
// discovery order. The section containing the active link is expanded.
func (s *Server) buildNav(active string) []navSection {
	sections := []navSection{{
		Title: "Status",
		Open:  true,
		Links: []navLink{{Label: "Overview", Href: "/"}},
	}}
	index := map[string]int{"Status": 0}

	for _, m := range s.manifests {
		for _, entry := range m.Nav {
			i, ok := index[entry.Section]
			if !ok {
				i = len(sections)
				index[entry.Section] = i
				sections = append(sections, navSection{Title: entry.Section})
			}
			sections[i].Links = append(sections[i].Links, navLink{
				Label: entry.Label,
				Href:  pluginHref(m.ID, entry.Path),
			})
		}
	}

	// Core sections first in canonical order, extension sections after by title.
	// Stable so link order within a section (id-sorted discovery order) is kept.
	sort.SliceStable(sections, func(a, b int) bool {
		ra, ca := coreRank(sections[a].Title)
		rb, cb := coreRank(sections[b].Title)
		if ca != cb {
			return ca // a core, b not → a first
		}
		if ca {
			return ra < rb
		}
		return sections[a].Title < sections[b].Title
	})

	for si := range sections {
		for li := range sections[si].Links {
			if isActive(active, sections[si].Links[li].Href) {
				sections[si].Links[li].Active = true
				sections[si].Open = true
			}
		}
	}
	return sections
}

// pluginHref is the shell-side URL for a plugin page: the /plugins/<id>/ mount
// joined with the manifest's (mount-relative) nav path.
func pluginHref(id, navPath string) string {
	return "/plugins/" + id + "/" + strings.TrimPrefix(navPath, "/")
}

func isActive(current, href string) bool {
	if href == "/" {
		return current == "/"
	}
	return current == href || strings.HasPrefix(current, href)
}

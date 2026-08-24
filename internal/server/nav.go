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

// buildNav assembles the sidebar for the current path. The shell's own pages come
// first (ADR-009 §3): the Status baseline and the auth surface (Password), which
// exist without any plugin — so a fresh device can always reach them. Plugins are
// then grouped under their manifest's nav.section. Sections are ordered by the
// core taxonomy (ADR-009 §2, §5): core sections first in canonical order,
// plugin-introduced sections after by title — deterministic regardless of plugin
// discovery order. The section containing the active link is expanded; Status
// stays open by default.
func (s *Server) buildNav(active string) []navSection {
	sections := make([]navSection, 0, len(coreSectionOrder))
	index := map[string]int{}
	add := func(section, label, href string) {
		i, ok := index[section]
		if !ok {
			i = len(sections)
			index[section] = i
			sections = append(sections, navSection{Title: section})
		}
		sections[i].Links = append(sections[i].Links, navLink{Label: label, Href: href})
	}

	// Shell-owned pages (ADR-009 §3): the read-only baseline and the auth surface.
	add("Status", "Overview", "/")
	add("System", "Password", "/system/password")

	// Plugin-contributed pages, in discovery (id-sorted) order.
	for _, m := range s.manifests {
		for _, entry := range m.Nav {
			add(entry.Section, entry.Label, pluginHref(m.ID, entry.Path))
		}
	}

	// Core sections first in canonical order, extension sections after by title.
	// Stable so link order within a section (built-ins first, then id-sorted
	// discovery order) is kept.
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
		if sections[si].Title == "Status" {
			sections[si].Open = true
		}
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

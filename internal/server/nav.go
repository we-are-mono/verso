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
	Icon   string // sidebar basic rows carry an icon; advanced text links leave it empty
	Active bool
	// Optional trailing detail on the right of a basic row — a short word or count with an
	// optional leading dot; Variant tints it ("good" = green). A couple of examples today;
	// any row can grow one later.
	Detail  string
	Dot     bool
	Variant string
}

// navModel is the device-first sidebar: a few everyday "basic" rows on top, then the
// technical pages grouped under a collapsible "Advanced settings" seam. System is a
// stable top-level destination of its own: the shell owns its five-page frame while the
// General page is supplied by the bundled System plugin. The remaining manifest-driven
// sections live under Advanced. The everyday rows are the plain-language destinations a
// non-technical person reaches for (placeholders until their pages exist).
type navModel struct {
	Basic    []navLink
	Advanced []navGroup
}

// navGroup is one titled cluster inside the Advanced seam.
type navGroup struct {
	Title string
	Links []navLink
}

// buildSidebar assembles the device-first sidebar for the current path: the everyday
// basic rows, then the manifest-driven sections (via buildNav) as the Advanced groups.
func (s *Server) buildSidebar(active string) navModel {
	basic := func(label, icon, href string) navLink {
		return navLink{Label: label, Icon: icon, Href: href, Active: isActive(active, href)}
	}
	m := navModel{Basic: []navLink{
		basic("Home", "house", "/"),
		{Label: "Internet", Icon: "globe", Href: "#", Detail: "Online", Dot: true, Variant: "good"},
		{Label: "Devices", Icon: "devices", Href: "#", Detail: "9"},
		basic("Wi-Fi", "wifi", "#"),
		basic("Family", "users", "#"),
		basic("Safety", "shield", "#"),
		basic("System", "settings", "/system/general"),
	}}
	// System's default destination is General, but the row represents the whole
	// five-page domain and remains selected while any System subpage is open.
	m.Basic[len(m.Basic)-1].Active = active == "/system" || strings.HasPrefix(active, "/system/")
	for _, sec := range s.buildNav(active) {
		if sec.Title == "Status" || sec.Title == "System" {
			continue // Home and System are first-class destinations above the seam
		}
		m.Advanced = append(m.Advanced, navGroup{Title: sec.Title, Links: sec.Links})
	}
	return m
}

// buildNav assembles the sidebar for the current path. The shell's own pages come
// first (ADR-009 §3): the Status baseline and the auth surface (Password), which
// exist without any plugin — so a fresh device can always reach them. Plugins are
// then grouped under their manifest's nav.section. Sections are ordered by the
// core taxonomy (ADR-009 §2, §5): core sections first in canonical order,
// plugin-introduced sections after by title — deterministic regardless of plugin
// discovery order. The section containing the active page is marked Open — the
// top nav highlights it (and marks its active dropdown entry).
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

	// Shell-owned pages (ADR-009 §3): the read-only baseline, the auth surface,
	// and the plugin-management surface (ADR-011).
	add("Status", "Overview", "/")
	add("System", "General", "/system/general")
	add("System", "Access", "/system/access")
	add("System", "Packages", "/system/packages")
	add("System", "Services", "/system/services")
	add("System", "Maintenance", "/system/maintenance")

	// Plugin-contributed pages, in discovery (id-sorted) order. Only plugins
	// whose socket answers contribute rows: a menu entry that leads to
	// "unavailable" is a dead door, and an installed-but-off plugin is the
	// management page's business (ADR-011). Direct URLs still answer — with
	// the notice and its way back on — so nothing is unreachable, just unlisted.
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			// The bundled System plugin fills the shell's fixed General slot. Its
			// manifest entry is needed for discovery, but must not duplicate the
			// stable shell-owned navigation row above.
			if m.ID == "system" && entry.Section == "System" {
				continue
			}
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

	// Only the most specific match lights up. A plugin's root link (…/, its index
	// subpage) is a prefix of every sibling subpage, so on a deeper page several
	// links match; the longest matching href is the real destination.
	bestSi, bestLi, bestLen := -1, -1, -1
	for si := range sections {
		for li := range sections[si].Links {
			if h := sections[si].Links[li].Href; isActive(active, h) && len(h) > bestLen {
				bestSi, bestLi, bestLen = si, li, len(h)
			}
		}
	}
	if bestSi >= 0 {
		sections[bestSi].Links[bestLi].Active = true
		sections[bestSi].Open = true
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

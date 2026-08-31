// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"sort"
	"strconv"
	"strings"
)

// coreSectionOrder is the shell-owned core taxonomy (ADR-009 §2): these sections
// render first, in exactly this order, ahead of any plugin-introduced section.
// Status is served by the shell itself; Network, Security and System are backed
// by bundled first-party plugins. Membership here is a nav-ordering guarantee, not
// a mechanism — every configuring section is still an ADR-006 plugin.
var coreSectionOrder = []string{"Status", "Network", "Security", "System"}

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
	// PluginID names the plugin that authored this entry's label ("" for a
	// shell-owned link), so the label is localized from that plugin's catalog
	// rather than the shell's base (ADR-012 §5).
	PluginID string
	// Optional trailing detail on the right of a basic row — a short word or count with an
	// optional leading dot; Variant tints it (the tone vocabulary, "success" = green).
	// A couple of examples today; any row can grow one later.
	Detail  string
	Dot     bool
	Variant string
}

// navModel is the device-first sidebar: a few everyday "basic" rows on top, then the
// technical pages grouped under a collapsible "Advanced settings" seam. Security and
// System are stable top-level destinations of their own — System's frame combines live
// plugin registrations with the shell-owned administration pages, Security is one
// plugin's domain — so neither also appears under Advanced. The remaining
// manifest-driven sections live there. The everyday rows are the plain-language
// destinations a non-technical person reaches for.
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
// Labels are localized at the display edge (ADR-012): shell-owned labels and the
// section titles from base (tr), each plugin's label from that plugin's catalog
// (pluginTr(id)). The section titles buildNav uses for ordering and filtering stay
// English internally.
func (s *Server) buildSidebar(active string, tr func(string) string, pluginTr func(id string) func(string) string) navModel {
	basic := func(label, icon, href string) navLink {
		return navLink{Label: tr(label), Icon: icon, Href: href, Active: isActive(active, href)}
	}
	// Security and System are domain rows rather than page links: each leads to the
	// first live registered page filed under its section and stays Active anywhere
	// inside that domain, plugin URLs included. System also holds shell-owned pages,
	// so it always resolves somewhere; Security is one plugin, and leads nowhere
	// while that plugin is not answering.
	security := basic("Security", "shield", s.sectionHref("Security"))
	security.Active = s.isSectionPath("Security", active)
	system := basic("System", "settings", "/system")
	if pages := s.systemPages(active); len(pages) > 0 {
		system.Href = pages[0].Href
	}
	system.Active = s.isSystemPath(active)

	// Devices is a live row: its detail is how many are on the network right now,
	// counted from the kernel's neighbour table. A box that cannot answer shows
	// the row without a number rather than a stale or invented one.
	devices := basic("Devices", "devices", devicesPath)
	if online, ok := s.onlineDevices(); ok {
		devices.Detail = strconv.Itoa(online)
	}

	m := navModel{Basic: []navLink{
		basic("Home", "house", "/"),
		{Label: tr("Internet"), Icon: "globe", Href: "#", Detail: tr("Online"), Dot: true, Variant: "success"},
		devices,
		basic("Wi-Fi", "wifi", "#"),
		security,
		system,
	}}
	for _, sec := range s.buildNav(active) {
		if sec.Title == "Status" || sec.Title == "Security" || sec.Title == "System" {
			continue // Home, Security and System are first-class destinations above the seam
		}
		links := make([]navLink, len(sec.Links))
		for i, l := range sec.Links {
			if l.PluginID == "" {
				l.Label = tr(l.Label)
			} else {
				l.Label = pluginTr(l.PluginID)(l.Label)
			}
			links[i] = l
		}
		m.Advanced = append(m.Advanced, navGroup{Title: tr(sec.Title), Links: links})
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
	// pluginID is "" for a shell-owned link and the plugin id for a plugin's, so
	// buildSidebar localizes each label from the right catalog (ADR-012 §5).
	add := func(section, label, href, pluginID string) {
		i, ok := index[section]
		if !ok {
			i = len(sections)
			index[section] = i
			sections = append(sections, navSection{Title: section})
		}
		sections[i].Links = append(sections[i].Links, navLink{Label: label, Href: href, PluginID: pluginID})
	}

	// Shell-owned pages (ADR-009 §3): the read-only baseline, the auth surface,
	// and the plugin-management surface (ADR-011). Plugin-owned System pages are
	// added below from their manifests; General is not a shell-owned slot.
	add("Status", "Overview", "/", "")
	add("System", "Access", "/system/access", "")
	add("System", "Packages", "/system/packages", "")
	add("System", "Services", "/system/services", "")
	add("System", "Maintenance", "/system/maintenance", "")

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
			add(entry.Section, entry.Label, pluginHref(m.ID, entry.Path), m.ID)
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

// isSystemPath reports whether a request belongs to either a shell-owned
// System page or any manifest-registered System plugin page.
func (s *Server) isSystemPath(active string) bool {
	if active == "/system" || strings.HasPrefix(active, "/system/") {
		return true
	}
	return s.isSectionPath("System", active)
}

// isSectionPath reports whether a request belongs to any manifest-registered
// page of one nav section. Liveness is not required here: a direct URL to a
// stopped plugin still belongs visually to its domain while it explains that the
// plugin is unavailable.
func (s *Server) isSectionPath(section, active string) bool {
	for _, m := range s.manifestList() {
		for _, entry := range m.Nav {
			if entry.Section == section && isActive(active, pluginHref(m.ID, entry.Path)) {
				return true
			}
		}
	}
	return false
}

// sectionHref is a domain row's destination: the first live registered page
// filed under that section, in discovery (id-sorted) order. A section no
// answering plugin fills has no destination — the row leads nowhere rather than
// to a URL that reports the plugin is unavailable.
func (s *Server) sectionHref(section string) string {
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			if entry.Section == section {
				return pluginHref(m.ID, entry.Path)
			}
		}
	}
	return "#"
}

// liveSectionHref turns a section destination into a link only when something
// actually serves it. The sidebar's rows keep the placeholder "#" — a row with no
// href would collapse the list's rhythm — but a status tile has no such obligation
// and simply stops being a doorway.
func liveSectionHref(href string) string {
	if href == "#" {
		return ""
	}
	return href
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

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
	Icon   string
	Active bool
	// Mode is the reading this entry belongs to (ADR-015): "basic", "advanced",
	// or empty for both. It comes from the manifest and filters the row.
	Mode string
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

// navModel is the rail: one flat list of destinations, with subpages under
// the active row.
type navModel struct {
	Rows []navRow
}

// navRow is one destination in the rail. The row you are in opens its subpages
// beneath it, so the rail carries the whole path rather than handing the last
// step to a bar somewhere else on the page.
type navRow struct {
	Label  string
	Href   string
	Icon   string
	Active bool
	// Children are the active row's subpages, already localized. Only the active
	// row has them: a plugin declares its pages in the envelope it answers with,
	// so the shell knows them for the page in hand and for no other.
	Children []pageTab
	// Detail rides hard right — a count, a rate. Dot marks a state beside the
	// label instead, tinted by Variant ("success", "danger").
	Detail  string
	Dot     bool
	Variant string
	// PluginID names the plugin that authored this row's label ("" for a
	// shell-owned row), so the label is localized from that plugin's catalog
	// rather than the shell's base (ADR-012 §5).
	PluginID string
}

// railOrder is the order the design canvas gives the rail. Matching on the
// English label is what lets a plugin land in its designed place without the
// shell knowing anything about that plugin: the labels buildNav carries are
// still English at this point, localized only at the display edge below. A
// destination the canvas does not name follows these, keeping the core section
// order buildNav already sorted it into (ADR-009 §5), so a new plugin appears in
// the rail with no shell change.
var railOrder = []string{
	"Overview", "Devices", "Traffic", "Journal", "Interfaces", "Wireless",
	"Firewall", "DNS & DHCP", "Routing", "Tunnels", "Storage", "System",
}

// railRank returns a label's position in the designed order, and whether the
// canvas names it at all.
func railRank(label string) (int, bool) {
	for i, l := range railOrder {
		if l == label {
			return i, true
		}
	}
	return 0, false
}

// navIcon chooses a shell-owned glyph before labels are localized. Designed
// destinations have their own identity; other entries inherit their section's
// glyph unless the plugin names one in its manifest.
func navIcon(label, section string) string {
	switch label {
	case "Overview":
		return "house"
	case "Devices":
		return "phone"
	case "Traffic":
		return "activity"
	case "Journal":
		return "menu"
	case "Interfaces":
		return "ethernet-port"
	case "Wireless":
		return "wifi"
	case "Firewall":
		return "zone"
	case "DNS & DHCP":
		return "globe"
	case "Routing":
		return "route"
	case "Tunnels":
		return "network"
	case "Storage":
		return "hard-drive"
	case "System":
		return "settings"
	}
	switch section {
	case "Status":
		return "activity"
	case "Network":
		return "network"
	case "Security":
		return "zone"
	case "System":
		return "settings"
	default:
		return "menu"
	}
}

// buildSidebar assembles the rail for the current path: one flat list of
// destinations in the designed order, the active one carrying the subpages it
// opens. Labels are localized at the display edge (ADR-012): shell-owned labels
// from base (tr), each plugin's from that plugin's catalog (pluginTr(id)), which
// is why the ordering above happens while they are still English.
func (s *Server) buildSidebar(active, mode string, tr func(string) string, pluginTr func(id string) func(string) string, pages []pageTab) navModel {
	rows := []navRow{{Label: "Overview", Href: "/", Icon: navIcon("Overview", "Status"), Active: isActive(active, "/")}}

	// Devices is a live row: its detail is how many are on the network right now,
	// counted from the kernel's neighbour table. A box that cannot answer shows
	// the row without a number rather than a stale or invented one.
	devices := navRow{Label: "Devices", Href: devicesPath, Icon: navIcon("Devices", "Status"), Active: isActive(active, devicesPath)}
	if online, ok := s.onlineDevices(); ok {
		devices.Detail = strconv.Itoa(online)
	}
	rows = append(rows, devices)

	// Every section the manifests register becomes rows here — no titles between
	// them, because the rail is a list of places and not a taxonomy. Status is
	// already served by Overview above; System collapses to one row whose
	// subpages are the shell-owned pages and the plugin-owned ones together.
	for _, sec := range s.buildNav(active) {
		if sec.Title == "Status" {
			continue
		}
		if sec.Title == "System" {
			system := navRow{Label: "System", Href: "/system", Icon: navIcon("System", "System"), Active: s.isSystemPath(active)}
			if sysPages := s.systemPages(active, mode); len(sysPages) > 0 {
				system.Href = sysPages[0].Href
			}
			rows = append(rows, system)
			continue
		}
		for _, l := range sec.Links {
			rows = append(rows, navRow{Label: l.Label, Href: l.Href, Icon: l.Icon, Active: l.Active, PluginID: l.PluginID})
		}
	}

	// The designed order, with anything the canvas does not name kept in the
	// order buildNav produced. Stable, so discovery order still decides between
	// two rows the canvas is silent about.
	sort.SliceStable(rows, func(a, b int) bool {
		ra, oka := railRank(rows[a].Label)
		rb, okb := railRank(rows[b].Label)
		if oka != okb {
			return oka
		}
		if !oka {
			return false
		}
		return ra < rb
	})

	// The one open row carries the subpages, and it is the only row that can:
	// the shell learns a plugin's pages from the envelope it has just rendered.
	for i := range rows {
		if rows[i].Active {
			rows[i].Children = pages
			break
		}
	}
	for i := range rows {
		if rows[i].PluginID == "" {
			rows[i].Label = tr(rows[i].Label)
		} else {
			rows[i].Label = pluginTr(rows[i].PluginID)(rows[i].Label)
		}
	}
	return navModel{Rows: rows}
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
	// A link's PluginID is "" for a shell-owned page and the plugin id for a
	// plugin's, so buildSidebar localizes each label from the right catalog
	// (ADR-012 §5).
	add := func(section string, link navLink) {
		if link.Icon == "" {
			link.Icon = navIcon(link.Label, section)
		}
		i, ok := index[section]
		if !ok {
			i = len(sections)
			index[section] = i
			sections = append(sections, navSection{Title: section})
		}
		sections[i].Links = append(sections[i].Links, link)
	}

	// Shell-owned pages (ADR-009 §3): the read-only baseline, the auth surface,
	// and the plugin-management surface (ADR-011). Plugin-owned System pages are
	// added below from their manifests; General is not a shell-owned slot.
	add("Status", navLink{Label: "Overview", Href: "/"})
	add("System", navLink{Label: "Access", Href: "/system/access"})
	add("System", navLink{Label: "Packages", Href: "/system/packages"})
	add("System", navLink{Label: "Services", Href: "/system/services"})
	add("System", navLink{Label: "Maintenance", Href: "/system/maintenance"})

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
			add(entry.Section, navLink{
				Label:    entry.Label,
				Href:     pluginHref(m.ID, entry.Path),
				Icon:     entry.Icon,
				Mode:     entry.Mode,
				PluginID: m.ID,
			})
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

// navLabelHref resolves a particular destination without confusing it with
// another plugin in the same section. Missing destinations stay unlinked.
func (s *Server) navLabelHref(label string) string {
	for _, manifest := range s.manifestList() {
		for _, entry := range manifest.Nav {
			if entry.Label == label && s.probe(manifest.Socket) {
				return pluginHref(manifest.ID, entry.Path)
			}
		}
	}
	return ""
}
